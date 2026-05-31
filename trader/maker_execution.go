package trader

import (
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/order"
)

func (crypto *Crypto) submitMakerEntry(
	symbol string,
	notional float64,
	quote broker.Quote,
	opportunity opportunity,
	playbook string,
	spreadBPS float64,
	feePct float64,
) error {
	limitPrice := makerLimitPrice(quote)

	if limitPrice <= 0 {
		return fmt.Errorf("maker limit price unavailable for %s", symbol)
	}

	preflight := broker.Buy{
		Symbol:    symbol,
		Notional:  notional,
		Quote:     quote,
		Execution: crypto.scopedRuntime().Execution,
	}

	if err := preflight.PreflightGates(); err != nil {
		return err
	}

	priceDecimalsValue, hasPriceDecimals := priceDecimals(symbol)
	makerOrder := broker.Maker{
		Symbol:           symbol,
		LimitPrice:       limitPrice,
		Notional:         notional,
		FeePct:           feePct,
		PriceDecimals:    priceDecimalsValue,
		HasPriceDecimals: hasPriceDecimals,
		Execution:        crypto.scopedRuntime().Execution,
	}

	if crypto.live != nil {
		return crypto.submitMakerEntryLive(makerOrder, quote, opportunity, playbook, spreadBPS)
	}

	return crypto.submitMakerEntryPaper(makerOrder, quote, opportunity, playbook, spreadBPS)
}

func (crypto *Crypto) submitMakerEntryPaper(
	makerOrder broker.Maker,
	quote broker.Quote,
	opportunity opportunity,
	playbook string,
	spreadBPS float64,
) error {
	clOrdID, err := makerOrder.SubmitPaper(crypto.wallet)

	if err != nil {
		if clOrdID != "" {
			intent := makerEntryIntent(makerOrder, quote, opportunity, playbook, spreadBPS)
			crypto.paper.trackEntry(clOrdID, makerOrder.Symbol, intent)
			crypto.paper.EnqueueReject(clOrdID, err.Error())
			crypto.publishMakerSubmit(makerOrder.Symbol, opportunity, playbook, clOrdID, makerOrder.Notional, spreadBPS, false)
		}

		if clOrdID == "" {
			errnie.Error(fmt.Errorf("maker submit paper for %s: %w", makerOrder.Symbol, err))
		}

		return err
	}

	postedAt := time.Now()
	intent := makerEntryIntent(makerOrder, quote, opportunity, playbook, spreadBPS)
	crypto.paper.trackEntry(clOrdID, makerOrder.Symbol, intent)
	crypto.publishMakerSubmit(makerOrder.Symbol, opportunity, playbook, clOrdID, makerOrder.Notional, spreadBPS, false)

	entry := &restingMakerEntry{
		clOrdID:     clOrdID,
		symbol:      makerOrder.Symbol,
		maker:       makerOrder,
		tracker:     broker.NewMakerQueueTracker(makerOrder.Symbol, makerOrder.LimitPrice, postedAt, quote.BidDepth),
		intent:      intent,
		opportunity: opportunity,
		playbook:    playbook,
		spreadBPS:   spreadBPS,
		postedAt:    postedAt,
	}
	crypto.makers.track(entry)

	return nil
}

func (crypto *Crypto) submitMakerEntryLive(
	makerOrder broker.Maker,
	quote broker.Quote,
	opportunity opportunity,
	playbook string,
	spreadBPS float64,
) error {
	clOrdID, err := order.NextClOrdID()

	if err != nil {
		return fmt.Errorf("generate cl_ord_id: %w", err)
	}

	makerOrder.ClOrdID = clOrdID
	intent := makerEntryIntent(makerOrder, quote, opportunity, playbook, spreadBPS)
	crypto.live.trackEntry(clOrdID, makerOrder.Symbol, intent)

	if err := makerOrder.SubmitLive(crypto.live.Router(), crypto.wallet); err != nil {
		crypto.live.dropIntent(clOrdID, makerOrder.Symbol)
		releaseEntryReservation(crypto.wallet, makerOrder.Notional)
		crypto.publishAudit("order_reject", makerOrder.Symbol, err.Error(), map[string]any{
			"cl_ord_id":  clOrdID,
			"phase":      "publish",
			"entry_type": "maker",
			"live":       true,
		})

		return err
	}

	postedAt := time.Now()
	entry := &restingMakerEntry{
		clOrdID:     clOrdID,
		symbol:      makerOrder.Symbol,
		maker:       makerOrder,
		tracker:     broker.NewMakerQueueTracker(makerOrder.Symbol, makerOrder.LimitPrice, postedAt, quote.BidDepth),
		intent:      intent,
		opportunity: opportunity,
		playbook:    playbook,
		spreadBPS:   spreadBPS,
		postedAt:    postedAt,
	}
	crypto.makers.track(entry)
	crypto.publishMakerSubmit(makerOrder.Symbol, opportunity, playbook, clOrdID, makerOrder.Notional, spreadBPS, true)

	return nil
}

func (crypto *Crypto) tryPaperMakerFills() {
	if crypto.live != nil || crypto.paper == nil || crypto.makers == nil {
		return
	}

	crypto.makerFillMu.Lock()
	defer crypto.makerFillMu.Unlock()

	for _, entry := range crypto.makers.pendingPaperEntries() {
		if entry == nil {
			continue
		}

		fill, err := entry.maker.BuildPaperFill(entry.tracker.Context())

		if err != nil {
			continue
		}

		crypto.makers.drop(entry.clOrdID, entry.symbol)
		crypto.paper.ScheduleFill(fill)
	}
}

func (crypto *Crypto) advanceMakerFallback() {
	if crypto.makers == nil {
		return
	}

	crypto.makerFillMu.Lock()
	defer crypto.makerFillMu.Unlock()

	for _, entry := range crypto.makers.advanceWaitTicks(crypto.live != nil) {
		crypto.fallbackMakerToTaker(entry)
	}
}

func (crypto *Crypto) fallbackMakerToTaker(entry *restingMakerEntry) {
	if entry == nil {
		return
	}

	if crypto.live != nil && entry.orderID != "" {
		cancelErr := crypto.live.Router().Publish(order.CancelOrder(entry.orderID, ""))

		if cancelErr != nil {
			errnie.Error(cancelErr)
		}
	}

	crypto.makers.drop(entry.clOrdID, entry.symbol)

	if crypto.live != nil {
		crypto.live.dropIntent(entry.clOrdID, entry.symbol)
	}

	if crypto.paper != nil {
		crypto.paper.dropIntent(entry.clOrdID, entry.symbol)
	}

	releaseEntryReservation(crypto.wallet, entry.maker.Notional)

	fallbackLast := entry.intent.quote.Last

	if fallbackLast <= 0 {
		fallbackLast = entry.maker.LimitPrice
	}

	buy := broker.Buy{
		Symbol:    entry.symbol,
		Notional:  entry.maker.Notional,
		Quote:     crypto.quotes.snapshot(entry.symbol, fallbackLast),
		FeePct:    crypto.takerFeePct(entry.symbol),
		Execution: crypto.scopedRuntime().Execution,
	}

	crypto.publishAudit("maker_fallback", entry.symbol, "maker queue timeout", map[string]any{
		"cl_ord_id":   entry.clOrdID,
		"limit_price": entry.maker.LimitPrice,
		"wait_ticks":  entry.waitTicks,
		"live":        crypto.live != nil,
	})

	if err := crypto.submitEntry(buy, entry.opportunity, entry.playbook, entry.spreadBPS); err != nil {
		errnie.Error(err)
	}
}

func makerEntryIntent(
	makerOrder broker.Maker,
	quote broker.Quote,
	opportunity opportunity,
	playbook string,
	spreadBPS float64,
) orderIntent {
	return orderIntent{
		kind:           "entry",
		entryType:      "maker",
		symbol:         makerOrder.Symbol,
		playbook:       playbook,
		notional:       makerOrder.Notional,
		quote:          quote,
		feePct:         makerOrder.FeePct,
		spreadBPS:      spreadBPS,
		score:          opportunity.Score,
		names:          opportunity.Names,
		trigger:        opportunity.Trigger,
		hasLotDecimals: lotDecimalsKnown(makerOrder.Symbol),
		lotDecimals:    lotDecimals(makerOrder.Symbol),
	}
}

func makerLimitPrice(quote broker.Quote) float64 {
	if quote.Bid > 0 {
		return quote.Bid
	}

	if quote.Last > 0 {
		return quote.Last
	}

	return 0
}

func (crypto *Crypto) publishMakerSubmit(
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
		"entry_type":   "maker",
		"live":         live,
	})
}

func (crypto *Crypto) makerFeePct(symbol string) float64 {
	catalog := market.Catalog()

	if catalog != nil {
		return catalog.MakerFeePercent(symbol)
	}

	return config.System.MakerFeePct
}

func priceDecimals(symbol string) (int, bool) {
	catalog := market.Catalog()

	if catalog == nil {
		return 0, false
	}

	pair := catalog.Lookup(symbol)

	if pair == nil || pair.PairDecimals <= 0 {
		return 0, false
	}

	return pair.PairDecimals, true
}
