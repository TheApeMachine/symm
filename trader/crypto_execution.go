package trader

import (
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/order"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/trader/economics"
	"github.com/theapemachine/symm/wallet"
)

func (crypto *Crypto) prepareEntryQuote(
	symbol string,
	last float64,
	measurements []perspectives.Measurement,
) broker.Quote {
	quote := crypto.quotes.snapshot(symbol, last)

	if !config.System.ExecutionStressEnabled {
		return quote
	}

	return economics.StressQuote(quote, economics.AdverseSelectionBPS(measurements), broker.StressRegimeFrom(measurements))
}

func (crypto *Crypto) hasPendingEntry(symbol string) bool {
	if crypto.makers != nil && crypto.makers.HasPending(symbol) {
		return true
	}

	if crypto.live != nil && crypto.live.HasPendingEntry(symbol) {
		return true
	}

	return crypto.paper != nil && crypto.paper.HasPendingEntry(symbol)
}

func (crypto *Crypto) submitEntry(
	buy broker.Buy,
	opportunity opportunity,
	playbook string,
	spreadBPS float64,
) error {
	if crypto.live != nil {
		return crypto.submitEntryLive(buy, opportunity, playbook, spreadBPS)
	}

	return crypto.submitEntryPaper(buy, opportunity, playbook, spreadBPS)
}

func (crypto *Crypto) submitEntryLive(
	buy broker.Buy,
	opportunity opportunity,
	playbook string,
	spreadBPS float64,
) error {
	clOrdID, err := order.NextClOrdID()

	if err != nil {
		return fmt.Errorf("generate cl_ord_id: %w", err)
	}

	buy.ClOrdID = clOrdID
	crypto.live.trackEntry(clOrdID, buy.Symbol, entryIntent(buy, opportunity, playbook, spreadBPS))

	if err := buy.SubmitLive(crypto.live.Router(), crypto.wallet); err != nil {
		crypto.live.dropIntent(clOrdID, buy.Symbol)
		releaseEntryReservation(crypto.wallet, buy.Notional)
		crypto.publishAudit("order_reject", buy.Symbol, err.Error(), map[string]any{
			"cl_ord_id": clOrdID,
			"phase":     "publish",
			"live":      true,
		})

		return err
	}

	crypto.publishEntrySubmit(buy.Symbol, opportunity, playbook, clOrdID, buy.Notional, spreadBPS, true)

	return nil
}

func (crypto *Crypto) submitEntryPaper(
	buy broker.Buy,
	opportunity opportunity,
	playbook string,
	spreadBPS float64,
) error {
	clOrdID, err := buy.SubmitPaper(crypto.wallet)

	if err != nil {
		if clOrdID != "" {
			crypto.paper.trackEntry(clOrdID, buy.Symbol, entryIntent(buy, opportunity, playbook, spreadBPS))
			crypto.paper.EnqueueReject(clOrdID, err.Error())
			crypto.publishEntrySubmit(buy.Symbol, opportunity, playbook, clOrdID, buy.Notional, spreadBPS, false)
		}

		return nil
	}

	crypto.paper.trackEntry(clOrdID, buy.Symbol, entryIntent(buy, opportunity, playbook, spreadBPS))
	crypto.publishEntrySubmit(buy.Symbol, opportunity, playbook, clOrdID, buy.Notional, spreadBPS, false)

	fill, buildErr := buy.BuildPaperFill(crypto.wallet)

	if buildErr != nil {
		crypto.paper.dropIntent(clOrdID, buy.Symbol)
		releaseEntryReservation(crypto.wallet, buy.Notional)

		return buildErr
	}

	crypto.paper.ScheduleFill(fill)

	return nil
}

func (crypto *Crypto) submitExit(
	sell broker.Sell,
	binding wallet.PositionBinding,
	entry float64,
	reason string,
) error {
	if crypto.live != nil {
		return crypto.submitExitLive(sell, binding, entry, reason)
	}

	return crypto.submitExitPaper(sell, binding, entry, reason)
}

func (crypto *Crypto) submitExitLive(
	sell broker.Sell,
	binding wallet.PositionBinding,
	entry float64,
	reason string,
) error {
	lotDecimalsValue := binding.LotDecimals
	hasLot := binding.HasLotDecimals

	sell.HasLotDecimals = hasLot
	sell.LotDecimals = lotDecimalsValue

	clOrdID, err := order.NextClOrdID()

	if err != nil {
		return fmt.Errorf("generate cl_ord_id: %w", err)
	}

	sell.ClOrdID = clOrdID
	crypto.live.trackExit(clOrdID, exitIntent(sell, binding, entry, reason))

	if err := sell.SubmitLive(crypto.live.Router(), crypto.wallet); err != nil {
		crypto.live.dropIntent(clOrdID, sell.Symbol)
		crypto.publishAudit("order_reject", sell.Symbol, err.Error(), map[string]any{
			"cl_ord_id": clOrdID,
			"phase":     "publish",
			"live":      true,
		})

		return err
	}

	crypto.publishExitSubmit(sell.Symbol, binding.Playbook, clOrdID, reason, true)

	return nil
}

func (crypto *Crypto) submitExitPaper(
	sell broker.Sell,
	binding wallet.PositionBinding,
	entry float64,
	reason string,
) error {
	clOrdID, err := sell.SubmitPaper(crypto.wallet)

	if err != nil {
		if clOrdID != "" {
			crypto.paper.trackExit(clOrdID, exitIntent(sell, binding, entry, reason))
			crypto.paper.EnqueueReject(clOrdID, err.Error())
			crypto.publishExitSubmit(sell.Symbol, binding.Playbook, clOrdID, reason, false)
		}

		return nil
	}

	crypto.paper.trackExit(clOrdID, exitIntent(sell, binding, entry, reason))
	crypto.publishExitSubmit(sell.Symbol, binding.Playbook, clOrdID, reason, false)

	fill, buildErr := sell.BuildPaperFill(crypto.wallet)

	if buildErr != nil {
		crypto.paper.dropIntent(clOrdID, sell.Symbol)

		return buildErr
	}

	if fill.Qty <= 0 {
		crypto.paper.dropIntent(clOrdID, sell.Symbol)

		return nil
	}

	crypto.paper.ScheduleFill(fill)

	return nil
}

func entryIntent(
	buy broker.Buy,
	opportunity opportunity,
	playbook string,
	spreadBPS float64,
) orderIntent {
	return orderIntent{
		kind:           "entry",
		entryType:      "taker",
		symbol:         buy.Symbol,
		playbook:       playbook,
		notional:       buy.Notional,
		quote:          buy.Quote,
		feePct:         buy.FeePct,
		spreadBPS:      spreadBPS,
		score:          opportunity.Score,
		names:          opportunity.Names,
		trigger:        opportunity.Trigger,
		hasLotDecimals: lotDecimalsKnown(buy.Symbol),
		lotDecimals:    lotDecimals(buy.Symbol),
	}
}

func exitIntent(
	sell broker.Sell,
	binding wallet.PositionBinding,
	entry float64,
	reason string,
) orderIntent {
	return orderIntent{
		kind:        "exit",
		symbol:      sell.Symbol,
		playbook:    binding.Playbook,
		quote:       sell.Quote,
		feePct:      binding.TakerFeePct,
		entryPrice:  entry,
		exitReason:  reason,
		predictedAt: binding.PredictedAt,
	}
}

func (crypto *Crypto) intentSession(clOrdID string) (*orderSession, orderIntent, bool) {
	if crypto.live != nil {
		intent, ok := crypto.live.intentFor(clOrdID)

		if ok {
			return &crypto.live.orderSession, intent, true
		}
	}

	if crypto.paper != nil {
		intent, ok := crypto.paper.intentFor(clOrdID)

		if ok {
			return &crypto.paper.orderSession, intent, true
		}
	}

	return nil, orderIntent{}, false
}

func (crypto *Crypto) handleOrderAck(ack order.Ack) {
	if ack.Success {
		if ack.Method == order.MethodAddOrder && ack.Result.ClOrdID != "" && ack.Result.OrderID != "" {
			crypto.makers.bindOrderID(ack.Result.ClOrdID, ack.Result.OrderID)
		}

		return
	}

	session, intent, ok := crypto.intentSession(ack.Result.ClOrdID)

	if !ok {
		return
	}

	crypto.makers.drop(ack.Result.ClOrdID, intent.symbol)
	handleRejectAck(session, crypto.wallet, ack)
	crypto.publishAudit("order_reject", intent.symbol, ack.Error, map[string]any{
		"method":    ack.Method,
		"cl_ord_id": ack.Result.ClOrdID,
		"live":      crypto.live != nil,
	})
}

func (crypto *Crypto) handleOrderFill(fill order.Fill) {
	session, intent, ok := crypto.intentSession(fill.ClOrdID)

	if !ok {
		return
	}

	switch intent.kind {
	case "entry":
		crypto.makers.drop(fill.ClOrdID, intent.symbol)
		crypto.handleEntryFill(fill, intent, session)

		if order.OrderFillTerminal(fill) {
			session.dropIntent(fill.ClOrdID, intent.symbol)
		}
	case "exit":
		crypto.handleExitFill(fill, intent)
		session.dropIntent(fill.ClOrdID, intent.symbol)
	}
}

func (crypto *Crypto) handleEntryFill(
	fill order.Fill,
	intent orderIntent,
	session *orderSession,
) {
	if err := applyBuyFill(crypto.wallet, fill, intent); err != nil {
		errnie.Error(err)
		releaseEntryReservation(crypto.wallet, intent.notional)

		return
	}

	if session.entryBound(fill.ClOrdID) {
		return
	}

	session.markEntryBound(fill.ClOrdID)

	opportunity := opportunity{
		Symbol:  intent.symbol,
		Score:   intent.score,
		Names:   intent.names,
		Trigger: intent.trigger,
	}
	now := time.Now()
	entryLabel := economics.EntryLabel(
		intent.symbol, intent.playbook, "buy", intent.quote, intent.notional,
		fill.Price, intent.feePct, intent.spreadBPS, now,
	)
	crypto.completeEntry(
		intent.symbol, fill.Price, fill.Price, opportunity, intent.playbook, entryLabel, now, intent,
	)
	crypto.publishFill(fill)
}

func (crypto *Crypto) handleExitFill(fill order.Fill, intent orderIntent) {
	if err := applySellFill(crypto.wallet, fill); err != nil {
		errnie.Error(err)

		return
	}

	exitLabel := economics.ExitLabel(
		intent.symbol, intent.playbook, intent.entryPrice, fill.Price,
		intent.feePct, intent.spreadBPS, time.Now(),
	)
	crypto.completeExit(
		intent.symbol, intent.exitReason, exitLabel, fill,
		intent.entryPrice, intent.playbook, intent.predictedAt,
	)
	crypto.publishFill(fill)
}

func (crypto *Crypto) publishEntrySubmit(
	symbol string,
	opportunity opportunity,
	playbook, clOrdID string,
	notional, spreadBPS float64,
	live bool,
) {
	trigger := triggerLabel(opportunity.Trigger)

	crypto.publishAudit("entry_submit", symbol, trigger, map[string]any{
		"why":          trigger,
		"playbook":     playbook,
		"perspectives": opportunity.Names,
		"conviction":   opportunity.Score,
		"edge":         opportunity.Edge,
		"cl_ord_id":    clOrdID,
		"slot_eur":     notional,
		"spread_bps":   spreadBPS,
		"live":         live,
	})
}

func (crypto *Crypto) publishExitSubmit(
	symbol, playbook, clOrdID, reason string,
	live bool,
) {
	crypto.publishAudit("exit_submit", symbol, reason, map[string]any{
		"why":       reason,
		"playbook":  playbook,
		"cl_ord_id": clOrdID,
		"live":      live,
	})
}

func (crypto *Crypto) completeEntry(
	symbol string,
	last float64,
	fillPrice float64,
	opportunity opportunity,
	playbook string,
	entryLabel economics.Label,
	now time.Time,
	intent orderIntent,
) {
	feePct := intent.feePct

	if feePct <= 0 {
		feePct = crypto.takerFeePct(symbol)
	}

	crypto.economics.RecordEntry(entryLabel)
	crypto.wallet.BindPosition(baseOf(symbol), wallet.PositionBinding{
		Source:         "perspective",
		Playbook:       playbook,
		EntryScore:     opportunity.Score,
		PredictedAt:    now,
		DueAt:          now.Add(config.System.PerspectiveTTL),
		TakerFeePct:    feePct,
		HasLotDecimals: intent.hasLotDecimals,
		LotDecimals:    intent.lotDecimals,
	})
	crypto.positions.Open(symbol, positionState{
		Playbook:   playbook,
		EntryScore: opportunity.Score,
		Peak:       last,
		EntryAt:    now,
	})
	crypto.open.Add(1)
	crypto.tracker.Add(symbol)

	econCount, econMean := crypto.economics.PlaybookStats(playbook)
	crypto.publishAudit("entry", symbol, "perspective entry on "+triggerLabel(opportunity.Trigger), map[string]any{
		"why":                    triggerLabel(opportunity.Trigger),
		"conviction":             opportunity.Score,
		"edge":                   opportunity.Edge,
		"perspectives":           opportunity.Names,
		"playbook":               playbook,
		"fill_price":             fillPrice,
		"entry_type":             intent.entryType,
		"live":                   crypto.live != nil,
		"quote_age_ms":           entryLabel.QuoteAgeMS,
		"spread_bps":             entryLabel.SpreadBPS,
		"projected_slippage_bps": entryLabel.ProjectedSlippageBPS,
		"depth_coverage":         entryLabel.DepthCoverage,
		"playbook_econ_samples":  econCount,
		"playbook_econ_mean":     econMean,
	})
	crypto.publishWallet()
}

func (crypto *Crypto) completeExit(
	symbol string,
	reason string,
	exitLabel economics.Label,
	fill order.Fill,
	entryPrice float64,
	playbook string,
	predictedAt time.Time,
) {
	heldMS := int64(0)

	if !predictedAt.IsZero() {
		heldMS = time.Since(predictedAt).Milliseconds()
	}

	crypto.economics.RecordExit(exitLabel)
	crypto.positions.Close(symbol)
	crypto.open.Add(-1)
	crypto.tracker.Remove(symbol)

	realized := realizedReturn(entryPrice, fill.Price)
	crypto.publishAudit("exit", symbol, reason, map[string]any{
		"why":           reason,
		"actual_return": realized,
		"net_return":    exitLabel.NetReturn,
		"success":       exitLabel.NetReturn > 0,
		"held_ms":       heldMS,
		"playbook":      playbook,
		"live":          crypto.live != nil,
	})
	crypto.publishFill(fill)
	crypto.publishWallet()
}
