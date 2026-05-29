package trader

import (
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
		crypto.recordEntrySkip(prediction, "no_lead_measurement", nil)

		return
	}

	symbol := lead.Pairs[0].Wsname
	requirement := crypto.entryReturnRequirement(symbol, lead)

	pumpRegime := pumpRegimeOf(lead)

	if pumpRegime == "pump_fast" {
		peak := crypto.pumpPeak[symbol]

		if peak <= 0 {
			crypto.recordEntrySkip(
				prediction,
				"pump_no_peak",
				requirement.auditFields(symbol, predictedReturn),
			)

			return
		}

		retrace := (peak - lead.Last) / peak

		// Closer than PumpPullbackMin = chasing the vertical; deeper than
		// PumpPullbackMax = the leg is dead. Enter only the re-spike dip.
		if retrace < config.System.PumpPullbackMin ||
			retrace > config.System.PumpPullbackMax {
			fields := requirement.auditFields(symbol, predictedReturn)
			fields["retrace"] = retrace
			fields["peak"] = peak
			fields["last"] = lead.Last
			crypto.recordEntrySkip(prediction, "pump_chase_guard", fields)

			return
		}
	}

	edge := requirement.edge(predictedReturn)
	jointConfidence, sourceCount := engine.FuseMeasurements(prediction.Perspective.Measurements)
	entryFields := requirement.auditFields(symbol, predictedReturn)
	entryFields["confidence"] = prediction.Confidence
	entryFields["joint_confidence"] = jointConfidence
	entryFields["source_count"] = sourceCount
	entryFields["open_count"] = crypto.openCount()

	audit("trade_entry_eval", entryFields)

	if predictedReturn < requirement.requiredEdgeReturn {
		crypto.recordEntrySkip(
			prediction,
			"edge_below_threshold",
			requirement.auditFields(symbol, predictedReturn),
		)

		return
	}

	if predictedReturn < requirement.requiredRReturn {
		crypto.recordEntrySkip(
			prediction,
			"r_multiple_below_threshold",
			requirement.auditFields(symbol, predictedReturn),
		)

		return
	}

	if crypto.holdsSymbol(crypto.wallet, symbol) {
		crypto.recordEntrySkip(
			prediction,
			"already_open",
			requirement.auditFields(symbol, predictedReturn),
		)

		return
	}

	if err := crypto.preTradeGate(symbol, edge, jointConfidence); err != nil {
		fields := requirement.auditFields(symbol, predictedReturn)
		fields["error"] = err.Error()
		crypto.recordEntrySkip(prediction, "risk_gate", fields)

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
		fields := requirement.auditFields(symbol, predictedReturn)
		fields["slot_eur"] = slot
		fields["min_cost_eur"] = config.System.MinCostEUR
		fields["joint_confidence"] = jointConfidence
		fields["source_count"] = sourceCount
		crypto.recordEntrySkip(prediction, "slot_below_min", fields)

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
	takeProfitPrice := takeProfitPriceFor(lead, predictedReturn)

	buy := broker.Buy{
		Symbol:         symbol,
		Notional:       slot,
		Quote:          quote,
		StopPrice:      stopPrice,
		LimitBelowStop: stopLimit,
	}

	if crypto.wallet.Type == wallet.CryptoWallet {
		err := crypto.submitLiveEntry(
			prediction,
			lead,
			slot,
			predictedReturn,
			stopPrice,
			stopLimit,
			takeProfitPrice,
		)

		if err != nil {
			audit("trade_entry_error", map[string]any{
				"symbol":   symbol,
				"slot_eur": slot,
				"error":    err.Error(),
			})
		}

		return
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
		crypto.recordEntrySkip(
			prediction,
			"empty_fill",
			requirement.auditFields(symbol, predictedReturn),
		)

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
	} else {
		if stopPrice > 0 {
			crypto.forecasts.RegisterStop(symbol, stopPrice)
		}

		if takeProfitPrice > 0 {
			crypto.forecasts.RegisterTakeProfit(symbol, takeProfitPrice)
		}
	}

	crypto.recordEntryPnL(symbol, fill.Price)
	crypto.pool.CreateBroadcastGroup("executions", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: fill,
	})

	crypto.broadcasts["ui"].Send(&qpool.QValue[any]{Value: map[string]any{
		"event":       "trade_entry",
		"ts":          time.Now().UTC().Format(time.RFC3339Nano),
		"symbol":      symbol,
		"side":        fill.Side,
		"qty":         fill.Qty,
		"price":       fill.Price,
		"slot":        slot,
		"edge":        edge,
		"take_profit": takeProfitPrice,
	}})

	fillFields := requirement.auditFields(symbol, predictedReturn)
	fillFields["side"] = fill.Side
	fillFields["qty"] = fill.Qty
	fillFields["price"] = fill.Price
	fillFields["slot_eur"] = slot
	fillFields["take_profit"] = takeProfitPrice
	fillFields["confidence"] = prediction.Confidence
	fillFields["source_count"] = sourceCount
	fillFields["dominant_source"] = dominantSource(prediction.Perspective.Measurements)
	fillFields["contributions"] = sourceContributions(prediction.Perspective.Measurements)
	fillFields["perspective_type"] = uint8(prediction.Perspective.Type)
	fillFields["balance_eur"] = crypto.wallet.BalanceCopy()
	fillFields["reserved_eur"] = crypto.wallet.ReservedCopy()
	fillFields["open_count"] = crypto.openCount()

	audit("trade_entry_fill", fillFields)

	crypto.sendWallet()
}

func takeProfitPriceFor(measurement engine.Measurement, predictedReturn float64) float64 {
	anchor := measurement.AnchorPrice()

	if anchor <= 0 || predictedReturn <= 0 || config.System.TakeProfitCapture <= 0 {
		return 0
	}

	return anchor * (1 + predictedReturn*config.System.TakeProfitCapture)
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
