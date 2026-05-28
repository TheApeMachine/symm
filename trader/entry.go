package trader

import (
	"strings"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/wallet"
)

func (crypto *Crypto) tryEnter(
	prediction engine.Prediction,
	predictedReturn float64,
) {
	lead, ok := prediction.LeadMeasurement()

	if !ok {
		audit("trade_entry_skip", map[string]any{
			"reason": "no_lead_measurement",
		})

		return
	}

	symbol := lead.Pairs[0].Wsname
	friction := entryFrictionReturn(lead)

	pumpRegime := pumpRegimeOf(lead)

	if pumpRegime == "pump_fast" {
		peak := crypto.pumpPeak[symbol]

		if peak <= 0 {
			audit("trade_entry_skip", map[string]any{
				"symbol": symbol,
				"reason": "pump_no_peak",
			})

			return
		}

		retrace := (peak - lead.Last) / peak

		// Closer than PumpPullbackMin = chasing the vertical; deeper than
		// PumpPullbackMax = the leg is dead. Enter only the re-spike dip.
		if retrace < config.System.PumpPullbackMin ||
			retrace > config.System.PumpPullbackMax {
			audit("trade_entry_skip", map[string]any{
				"symbol":  symbol,
				"reason":  "pump_chase_guard",
				"retrace": retrace,
				"peak":    peak,
				"last":    lead.Last,
			})

			return
		}
	}

	edge := predictedReturn - friction
	jointConfidence, sourceCount := engine.FuseMeasurements(prediction.Perspective.Measurements)

	audit("trade_entry_eval", map[string]any{
		"symbol":           symbol,
		"predicted_return": predictedReturn,
		"friction":         friction,
		"edge":             edge,
		"confidence":       prediction.Confidence,
		"joint_confidence": jointConfidence,
		"source_count":     sourceCount,
		"open_count":       crypto.openCount(),
	})

	if predictedReturn < config.System.EntryEdgeMultiple*friction {
		audit("trade_entry_skip", map[string]any{
			"symbol":            symbol,
			"reason":            "edge_below_threshold",
			"edge":              edge,
			"predicted_return":  predictedReturn,
			"friction":          friction,
			"required_multiple": config.System.EntryEdgeMultiple,
		})

		return
	}

	stopFraction := crypto.stopFractionFor(symbol)

	if predictedReturn < config.System.TakeProfitR*stopFraction {
		audit("trade_entry_skip", map[string]any{
			"symbol":           symbol,
			"reason":           "r_multiple_below_threshold",
			"predicted_return": predictedReturn,
			"stop_fraction":    stopFraction,
			"required_r":       config.System.TakeProfitR,
		})

		return
	}

	if crypto.holdsSymbol(crypto.wallet, symbol) {
		audit("trade_entry_skip", map[string]any{
			"symbol": symbol,
			"reason": "already_open",
		})

		return
	}

	if err := crypto.preTradeGate(symbol, edge, jointConfidence); err != nil {
		audit("trade_entry_skip", map[string]any{
			"symbol": symbol,
			"reason": "risk_gate",
			"error":  err.Error(),
		})

		return
	}

	slot := crypto.kellySizer.SlotEUR(
		crypto.wallet.AvailableEUR(),
		engine.PerspectiveSource(prediction.Perspective.Type),
		engine.FeedbackRegime(prediction.Perspective, lead),
		jointConfidence,
		crypto.forecasts.RunningMeanError(),
	)

	// Pump-regime slots are sized down before the single MinCostEUR gate: a
	// pump position risks PumpSizeFraction of the normal slot, and the gate
	// then runs once against the reduced notional.
	if pumpRegime != "" {
		slot *= config.System.PumpSizeFraction
	}

	if slot < config.System.MinCostEUR {
		audit("trade_entry_skip", map[string]any{
			"symbol":           symbol,
			"reason":           "slot_below_min",
			"slot_eur":         slot,
			"min_cost_eur":     config.System.MinCostEUR,
			"joint_confidence": jointConfidence,
			"source_count":     sourceCount,
		})

		return
	}

	quoteAt := time.Time{}

	if _, _, _, at, ok := crypto.forecasts.LastQuote(symbol); ok {
		quoteAt = at
	}

	quote := broker.Quote{
		Last: lead.Last,
		Bid:  lead.Bid,
		Ask:  lead.Ask,
		At:   quoteAt,
	}

	stopPrice, stopLimit := crypto.stopPricesFor(lead, predictedReturn)

	buy := broker.Buy{
		Symbol:         symbol,
		Notional:       slot,
		Quote:          quote,
		StopPrice:      stopPrice,
		LimitBelowStop: stopLimit,
	}

	fill, err := buy.FillPaper(crypto.wallet)

	if err != nil {
		audit("trade_entry_error", map[string]any{
			"symbol":   symbol,
			"slot_eur": slot,
			"error":    err.Error(),
		})

		return
	}

	if fill.Qty <= 0 {
		audit("trade_entry_skip", map[string]any{
			"symbol": symbol,
			"reason": "empty_fill",
		})

		return
	}

	position := wallet.PositionBinding{
		Source:      engine.PerspectiveSource(prediction.Perspective.Type),
		Regime:      pumpRegime,
		PredictedAt: prediction.PredictedAt,
		DueAt:       prediction.DueAt,
	}

	if lead.Pairs[0].LotDecimals > 0 {
		position.HasLotDecimals = true
		position.LotDecimals = lead.Pairs[0].LotDecimals
	}

	crypto.wallet.BindPosition(symbolBase(symbol), position)
	crypto.attachWalletMarks()

	if pumpRegime != "" {
		// Pump positions have no time gate (§15.3); the trailing stop is the
		// sole downside control. It ratchets its peak up with the spike and
		// fires PumpTrailPct (fast) / PumpSlowTrailPct (slow) off the high, or
		// at the PumpHardStopPct floor if the move reverses immediately.
		trail := config.System.PumpTrailPct

		if pumpRegime == "pump_slow" {
			trail = config.System.PumpSlowTrailPct
		}

		crypto.forecasts.RegisterTrailingStop(
			symbol, fill.Price*(1-config.System.PumpHardStopPct), trail,
		)
	} else if stopPrice > 0 {
		crypto.forecasts.RegisterStop(symbol, stopPrice)
	}

	crypto.recordEntryPnL(symbol, fill.Price)
	crypto.pool.CreateBroadcastGroup("executions", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: fill,
	})

	crypto.broadcasts["ui"].Send(&qpool.QValue[any]{Value: map[string]any{
		"event":  "trade_entry",
		"ts":     time.Now().UTC().Format(time.RFC3339Nano),
		"symbol": symbol,
		"side":   fill.Side,
		"qty":    fill.Qty,
		"price":  fill.Price,
		"slot":   slot,
		"edge":   edge,
	}})

	audit("trade_entry_fill", map[string]any{
		"symbol":           symbol,
		"side":             fill.Side,
		"qty":              fill.Qty,
		"price":            fill.Price,
		"slot_eur":         slot,
		"edge":             edge,
		"predicted_return": predictedReturn,
		"stop_fraction":    stopFraction,
		"confidence":       prediction.Confidence,
		"source_count":     sourceCount,
		"dominant_source":  dominantSource(prediction.Perspective.Measurements),
		"contributions":    sourceContributions(prediction.Perspective.Measurements),
		"perspective_type": uint8(prediction.Perspective.Type),
		"balance_eur":      crypto.wallet.BalanceCopy(),
		"reserved_eur":     crypto.wallet.ReservedCopy(),
		"open_count":       crypto.openCount(),
	})

	crypto.sendWallet()
}

func entryFrictionReturn(measurement engine.Measurement) float64 {
	feePct := config.System.TakerFeePct * 2

	if config.System.UseMakerEntries {
		feePct = config.System.MakerFeePct + config.System.TakerFeePct
	}

	feeReturn := feePct / 100
	spreadReturn := quoteSpreadBPS(
		measurementAnchorPrice(measurement),
		measurement.Bid,
		measurement.Ask,
	) / 10000

	return feeReturn + spreadReturn
}

func quoteSpreadBPS(last, bid, ask float64) float64 {
	if last <= 0 {
		last = bid

		if ask > 0 {
			last = ask
		}
	}

	if last <= 0 || bid <= 0 || ask <= 0 {
		return 0
	}

	return (ask - bid) / last * 10000
}

func measurementAnchorPrice(measurement engine.Measurement) float64 {
	return measurement.AnchorPrice()
}

// sourceContributions returns the per-source max confidence that fed the
// fusion behind this entry. It is the raw material for per-signal attribution:
// because FuseMeasurements collapses sources into one joint confidence, the
// individual contributions are otherwise lost the moment a trade fills. Logged
// on trade_entry_fill so analysis/attribution.py can join realized PnL back to
// the signals that authorized each position.
func sourceContributions(measurements []engine.Measurement) map[string]float64 {
	contributions := make(map[string]float64, len(measurements))

	for _, measurement := range measurements {
		if measurement.Confidence <= 0 || measurement.Source == "" {
			continue
		}

		if measurement.Confidence > contributions[measurement.Source] {
			contributions[measurement.Source] = measurement.Confidence
		}
	}

	return contributions
}

// dominantSource is the single source with the highest confidence among the
// fused measurements — a convenience tag for grouping trades by their primary
// driver without re-deriving it from the contributions map.
func dominantSource(measurements []engine.Measurement) string {
	best := ""
	bestConfidence := 0.0

	for _, measurement := range measurements {
		if measurement.Source == "" {
			continue
		}

		if measurement.Confidence > bestConfidence {
			bestConfidence = measurement.Confidence
			best = measurement.Source
		}
	}

	return best
}

func symbolBase(symbol string) string {
	base, _, _ := strings.Cut(symbol, "/")

	return base
}

func perspectiveType(measurement engine.Measurement) engine.PerspectiveType {
	switch measurement.Type {
	case engine.LeadLag:
		return engine.PerspectiveCrossAsset
	case engine.Sentiment:
		return engine.PerspectiveSentiment
	case engine.Flow, engine.DepthFlow:
		return engine.PerspectiveFlow
	default:
		return engine.PerspectiveMicrostructure
	}
}

func runwayForPerspective(perspective engine.Perspective) time.Duration {
	runway := time.Duration(0)

	for _, measurement := range perspective.Measurements {
		if measurement.Timeframe.End <= measurement.Timeframe.Start {
			continue
		}

		candidate := time.Duration(measurement.Timeframe.End-measurement.Timeframe.Start) * time.Second

		if candidate > runway {
			runway = candidate
		}
	}

	if runway > 0 {
		return runway
	}

	for _, measurement := range perspective.Measurements {
		switch measurement.Type {
		case engine.Flow, engine.DepthFlow:
			return config.System.FlowHoldBeforeExit
		case engine.Causal:
			return config.System.MinHoldBeforeRotate
		}
	}

	return config.System.ScalpHoldBeforeExit
}

func predictionDirection(perspective engine.Perspective) int {
	score := 0.0

	for _, measurement := range perspective.Measurements {
		score += measurement.Confidence * float64(measurementDirection(measurement))
	}

	if score < 0 {
		return -1
	}

	return 1
}

func measurementDirection(measurement engine.Measurement) int {
	switch measurement.Type {
	case engine.Dump:
		return -1
	default:
		return 1
	}
}

// pumpRegimeOf classifies a pump measurement into a fast or slow regime, or ""
// when it is not a pump signal. Routes off pumpdump's existing reasons
// (fast_pump / actual_pump / slow_breakout from BestVolumeSpike + SlowRVOL).
func pumpRegimeOf(measurement engine.Measurement) string {
	if measurement.Source != "pumpdump" {
		return ""
	}

	switch measurement.Reason {
	case "fast_pump":
		return "pump_fast"
	case "actual_pump", "slow_breakout":
		return "pump_slow"
	}

	return ""
}

func isPumpRegime(regime string) bool {
	return regime == "pump_fast" || regime == "pump_slow"
}
