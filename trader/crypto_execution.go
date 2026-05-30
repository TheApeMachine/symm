package trader

import (
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/order"
	"github.com/theapemachine/symm/trader/economics"
	"github.com/theapemachine/symm/wallet"
)

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
	crypto.live.trackEntry(clOrdID, buy.Symbol, orderIntent{
		kind:      "entry",
		symbol:    buy.Symbol,
		playbook:  playbook,
		notional:  buy.Notional,
		quote:     buy.Quote,
		feePct:    buy.FeePct,
		spreadBPS: spreadBPS,
		score:     opportunity.Score,
		names:     opportunity.Names,
		trigger:   opportunity.Trigger,
	})

	if err := buy.SubmitLive(crypto.live.Router(), crypto.wallet); err != nil {
		crypto.live.dropIntent(clOrdID, buy.Symbol)
		releaseEntryReservation(crypto.wallet, buy.Notional)

		return err
	}

	crypto.publishAudit("entry_submit", buy.Symbol, "live entry submitted", map[string]any{
		"playbook":   playbook,
		"cl_ord_id":  clOrdID,
		"slot_eur":   buy.Notional,
		"spread_bps": spreadBPS,
	})

	return nil
}

func (crypto *Crypto) submitExitLive(
	sell broker.Sell,
	binding wallet.PositionBinding,
	entry float64,
	reason string,
) error {
	lotDecimalsValue, hasLot := liveLotDecimals(sell.Symbol, orderIntent{
		hasLotDecimals: binding.HasLotDecimals,
		lotDecimals:    binding.LotDecimals,
	})

	sell.HasLotDecimals = hasLot
	sell.LotDecimals = lotDecimalsValue

	clOrdID, err := order.NextClOrdID()

	if err != nil {
		return fmt.Errorf("generate cl_ord_id: %w", err)
	}

	sell.ClOrdID = clOrdID
	crypto.live.trackExit(clOrdID, orderIntent{
		kind:        "exit",
		symbol:      sell.Symbol,
		playbook:    binding.Playbook,
		quote:       sell.Quote,
		feePct:      binding.TakerFeePct,
		spreadBPS:   crypto.quotes.spreadBPS(sell.Symbol),
		entryPrice:  entry,
		exitReason:  reason,
		predictedAt: binding.PredictedAt,
	})

	if err := sell.SubmitLive(crypto.live.Router(), crypto.wallet); err != nil {
		crypto.live.dropIntent(clOrdID, sell.Symbol)

		return err
	}

	crypto.publishAudit("exit_submit", sell.Symbol, reason, map[string]any{
		"playbook":  binding.Playbook,
		"cl_ord_id": clOrdID,
	})

	return nil
}

func (crypto *Crypto) handleLiveAck(ack order.Ack) {
	if ack.Success {
		return
	}

	crypto.live.handleRejectAck(crypto.wallet, ack)
	crypto.publishAudit("order_reject", "", ack.Error, map[string]any{
		"method":    ack.Method,
		"cl_ord_id": ack.Result.ClOrdID,
	})
}

func (crypto *Crypto) handleLiveFill(fill order.Fill) {
	intent, ok := crypto.live.intentFor(fill.ClOrdID)

	if !ok {
		return
	}

	crypto.live.dropIntent(fill.ClOrdID, intent.symbol)

	switch intent.kind {
	case "entry":
		crypto.handleLiveEntryFill(fill, intent)
	case "exit":
		crypto.handleLiveExitFill(fill, intent)
	}
}

func (crypto *Crypto) handleLiveEntryFill(fill order.Fill, intent orderIntent) {
	if err := crypto.live.applyBuyFill(crypto.wallet, fill, intent); err != nil {
		errnie.Error(err)
		releaseEntryReservation(crypto.wallet, intent.notional)

		return
	}

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
	crypto.completeEntry(intent.symbol, fill.Price, fill.Price, opportunity, intent.playbook, entryLabel, now)
	crypto.publishFill(fill)
}

func (crypto *Crypto) handleLiveExitFill(fill order.Fill, intent orderIntent) {
	if err := crypto.live.applySellFill(crypto.wallet, fill); err != nil {
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

func (crypto *Crypto) completeEntry(
	symbol string,
	last float64,
	fillPrice float64,
	opportunity opportunity,
	playbook string,
	entryLabel economics.Label,
	now time.Time,
) {
	feePct := crypto.takerFeePct(symbol)

	crypto.economics.RecordEntry(entryLabel)
	crypto.wallet.BindPosition(baseOf(symbol), wallet.PositionBinding{
		Source:         "perspective",
		Playbook:       playbook,
		EntryScore:     opportunity.Score,
		PredictedAt:    now,
		DueAt:          now.Add(config.System.PerspectiveTTL),
		TakerFeePct:    feePct,
		HasLotDecimals: lotDecimalsKnown(symbol),
		LotDecimals:    lotDecimals(symbol),
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
