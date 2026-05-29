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
	verdict engine.Verdict,
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
	entryFields["node"] = verdict.Node
	entryFields["pump_regime"] = pumpRegime

	audit("trade_entry_eval", entryFields)

	// No break-even friction gate here: the decision tree already authorized this
	// entry by reading the whole market story (see actOnPrediction). tryEnter's
	// remaining job is risk and sizing, not a second go/no-go on edge.

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
		prediction.Runway,
	)

	// Pump-regime slots are sized down: a pump position risks PumpSizeFraction
	// of the normal slot.
	if pumpRegime != "" {
		slot *= config.System.PumpSizeFraction
	}

	// The decision tree is the authority on whether to enter; Kelly only scales
	// the size. A cold Kelly (no settled feedback yet) returns zero, but that
	// must NOT silently veto the tree's decision -- floor the slot at the minimum
	// tradeable notional so the entry is always honored, and let Kelly scale it
	// up as it learns. Only an outright lack of cash can prevent the trade.
	if slot < config.System.MinCostEUR {
		slot = config.System.MinCostEUR
	}

	if crypto.wallet.AvailableEUR() < slot {
		fields := requirement.auditFields(symbol, predictedReturn)
		fields["slot_eur"] = slot
		fields["available_eur"] = crypto.wallet.AvailableEUR()
		crypto.recordEntrySkip(prediction, "insufficient_cash", fields)

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

	// Real per-pair taker fee, threaded into the paper fill so realized PnL is
	// charged the same fee the entry economics gated on, and stored on the
	// binding so the exit sell bills the matching fee.
	takerFeePct, _ := pairFeePcts(lead)

	buy := broker.Buy{
		Symbol:         symbol,
		Notional:       slot,
		Quote:          quote,
		StopPrice:      stopPrice,
		LimitBelowStop: stopLimit,
		FeePct:         takerFeePct,
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
		TakerFeePct: takerFeePct,
	}

	if lead.Pairs[0].LotDecimals > 0 {
		position.HasLotDecimals = true
		position.LotDecimals = lead.Pairs[0].LotDecimals
	}

	crypto.wallet.BindPosition(symbolBase(symbol), position)
	crypto.attachWalletMarks()

	// Track this prediction for runway-expiry exit. Done here, at the moment a
	// position is actually opened, rather than on every fresh forecast: the
	// prediction's DueAt matches the position binding's DueAt, so
	// settlePredictions/holdsPrediction can pair them, and the slice stays
	// bounded by open positions instead of the perspective firehose.
	crypto.predictions = append(crypto.predictions, &prediction)

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
	fillFields["node"] = verdict.Node
	fillFields["pump_regime"] = pumpRegime
	fillFields["taker_fee_pct"] = takerFeePct

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
	taker, maker := pairFeePcts(measurement)

	// Round trip = entry leg + exit leg. The exit is always a taker market sell;
	// the entry is taker unless maker entries are enabled.
	feePct := taker * 2

	if config.System.UseMakerEntries {
		feePct = maker + taker
	}

	feeReturn := feePct / 100
	spreadReturn := quoteSpreadBPS(
		measurementAnchorPrice(measurement),
		measurement.Bid,
		measurement.Ask,
	) / 10000

	return feeReturn + spreadReturn
}

/*
pairFeePcts returns the real per-pair taker and maker fee percents for the
measurement's pair, read from Kraken's fee schedule at the configured 30-day
volume tier. When the pair carries no schedule (e.g. the REST enrichment has not
run), the configured fallback fees are used. This is the single source of fee
truth for both the entry friction gate and the paper-fill PnL accounting.
*/
func pairFeePcts(measurement engine.Measurement) (taker, maker float64) {
	taker = config.System.TakerFeePct
	maker = config.System.MakerFeePct

	if len(measurement.Pairs) > 0 {
		pair := measurement.Pairs[0]
		taker = pair.TakerFeePctOr(config.System.Fee30DVolume, config.System.TakerFeePct)
		maker = pair.MakerFeePctOr(config.System.Fee30DVolume, config.System.MakerFeePct)
	}

	return taker, maker
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
