package trader

import (
	"strings"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
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
	edge := predictedReturn - friction

	audit("trade_entry_eval", map[string]any{
		"symbol":           symbol,
		"predicted_return": predictedReturn,
		"friction":         friction,
		"edge":             edge,
		"confidence":       prediction.Confidence,
		"joint_confidence": jointConfidence(prediction.Perspective),
		"open_count":       crypto.openCount(),
	})

	if edge <= 0 {
		audit("trade_entry_skip", map[string]any{
			"symbol": symbol,
			"reason": "edge_non_positive",
			"edge":   edge,
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

	jointConfidence, sourceCount := engine.FuseMeasurements(prediction.Perspective.Measurements)
	slot := crypto.kellySizer.SlotEUR(
		crypto.wallet.AvailableEUR(),
		engine.PerspectiveSource(prediction.Perspective.Type),
		lead.Regime,
		jointConfidence,
		crypto.forecasts.RunningMeanError(),
	)

	if slot < config.System.MinCostEUR {
		audit("trade_entry_skip", map[string]any{
			"symbol":          symbol,
			"reason":          "slot_below_min",
			"slot_eur":        slot,
			"min_cost_eur":    config.System.MinCostEUR,
			"joint_confidence": jointConfidence,
			"source_count":    sourceCount,
		})

		return
	}

	quote := broker.Quote{
		Last: lead.Last,
		Bid:  lead.Bid,
		Ask:  lead.Ask,
	}

	buy := broker.Buy{
		Symbol:   symbol,
		Notional: slot,
		Quote:    quote,
	}

	fill, err := buy.FillPaper(crypto.wallet)

	if err != nil {
		audit("trade_entry_error", map[string]any{
			"symbol": symbol,
			"slot_eur": slot,
			"error":  err.Error(),
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

	crypto.attachWalletMarks()
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
		"confidence":       prediction.Confidence,
		"source_count":     sourceCount,
		"balance_eur":      crypto.wallet.Balance,
		"reserved_eur":     crypto.wallet.ReservedEUR,
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

	return feeReturn + spreadReturn/2
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

func jointConfidence(perspective engine.Perspective) float64 {
	confidence, _ := engine.FuseMeasurements(perspective.Measurements)

	return confidence
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
