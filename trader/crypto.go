package trader

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/order"
	decision "github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/trader/economics"
	"github.com/theapemachine/symm/wallet"
)

/*
Crypto is the trade desk. Entry and exit verdicts come from the perspectives
system; the desk records readings, applies cross-section sizing, friction gates,
TTL, and pump trailing stops, then fills paper orders.
*/
type Crypto struct {
	ctx          context.Context
	cancel       context.CancelFunc
	pool         *qpool.Q
	measurements *qpool.Subscriber
	ui           *qpool.BroadcastGroup
	wallet       *wallet.Wallet
	tracker      *focus.Set
	story        *decision.Story
	positions    *positionBook
	mu           sync.RWMutex
	readings     map[string]map[perspectives.SourceType]timedMeasurement
	quotes       *quoteCache
	economics    *economics.Desk
	open         atomic.Int64
	auditSeq     atomic.Uint64
}

func NewCrypto(
	ctx context.Context,
	pool *qpool.Q,
	tradingWallet *wallet.Wallet,
	tracker *focus.Set,
) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:       ctx,
		cancel:    cancel,
		pool:      pool,
		wallet:    tradingWallet,
		tracker:   tracker,
		story:     decision.NewStory(),
		positions: newPositionBook(),
		quotes:    newQuoteCache(),
		economics: economics.NewDesk(),
		readings:  make(map[string]map[perspectives.SourceType]timedMeasurement),
	}

	group := pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	crypto.measurements = group.Subscribe("trader:measurements", 128)
	crypto.ui = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

	return crypto
}

func (crypto *Crypto) Tick() error {
	heartbeat := time.NewTicker(config.System.UIHeartbeatInterval)
	defer heartbeat.Stop()

	tickers := market.NewTickerSubscription(crypto.ctx, config.System.Symbols...)
	books := market.NewBookSubscription(
		crypto.ctx, config.System.BookDepthLevels, config.System.Symbols...,
	)

	for {
		select {
		case <-crypto.ctx.Done():
			return crypto.ctx.Err()
		case <-heartbeat.C:
			crypto.publishWallet()
		case row, ok := <-tickers:
			if !ok {
				tickers = nil

				continue
			}

			if row != nil {
				crypto.quotes.ingestTicker(*row)
			}
		case update, ok := <-books:
			if !ok {
				books = nil

				continue
			}

			if update != nil {
				crypto.quotes.ingestBook(*update)
			}
		case value, ok := <-crypto.measurements.Incoming:
			if !ok || value.Value == nil {
				continue
			}

			measurement, measurementOK := value.Value.(perspectives.Measurement)

			if !measurementOK || measurement.Symbol == "" {
				continue
			}

			crypto.record(measurement)
			crypto.evaluate(measurement.Symbol, measurement.Last)
		}
	}
}

func (crypto *Crypto) Close() error {
	crypto.cancel()
	return nil
}

func (crypto *Crypto) ResendWallet() {
	crypto.publishWallet()
}

func (crypto *Crypto) record(measurement perspectives.Measurement) {
	crypto.mu.Lock()
	defer crypto.mu.Unlock()

	set := crypto.readings[measurement.Symbol]

	if set == nil {
		set = make(map[perspectives.SourceType]timedMeasurement)
		crypto.readings[measurement.Symbol] = set
	}

	set[measurement.Source] = newTimedMeasurement(measurement, set[measurement.Source])
}

func (crypto *Crypto) evaluate(symbol string, last float64) {
	if last > 0 {
		crypto.resolveEconomics(symbol, last)
	}

	measurements := crypto.snapshot(symbol)

	if _, held := crypto.wallet.PositionBindingFor(baseOf(symbol)); held {
		crypto.manage(symbol, last, measurements)

		return
	}

	crypto.consider(symbol, last, measurements)
}

func (crypto *Crypto) snapshot(symbol string) []perspectives.Measurement {
	crypto.mu.RLock()
	defer crypto.mu.RUnlock()

	return snapshotTimedMeasurements(crypto.readings[symbol], time.Now())
}

func (crypto *Crypto) consider(symbol string, last float64, measurements []perspectives.Measurement) {
	entryDecisions := decision.Decisions(measurements, nil)
	crypto.story.RecordEntry(symbol, entryDecisions)

	opportunity, ok := crypto.entryOpportunity(symbol, measurements)

	if !ok {
		return
	}

	opportunity, ok = crypto.calibrateOpportunity(opportunity)

	if !ok {
		return
	}

	crypto.enter(symbol, last, opportunity)
}

func (crypto *Crypto) manage(symbol string, last float64, measurements []perspectives.Measurement) {
	base := baseOf(symbol)
	binding, _ := crypto.wallet.PositionBindingFor(base)

	if last > 0 {
		crypto.positions.UpdatePeak(symbol, last)
	}

	if crypto.positions.PumpTrailBreached(symbol, last) {
		crypto.exit(symbol, last, perspectives.ActionStopLoss, "pump trail breached")

		return
	}

	if time.Now().After(binding.DueAt) {
		crypto.exit(symbol, last, perspectives.ActionTakeProfit, "perspective TTL elapsed")

		return
	}

	observations := []perspectives.ObservationType{perspectives.ObservationHolding}
	softAllowed := time.Since(binding.PredictedAt) >= config.System.MinExhaustHold
	exitDecisions := decision.ExitDecisions(
		measurements, observations, binding.Playbook, softAllowed,
	)
	crypto.story.RecordExit(symbol, exitDecisions)

	action := decision.MostUrgentExit(exitDecisions)

	if action == nil {
		return
	}

	crypto.exit(symbol, last, *action, exitReason(*action))
}

func (crypto *Crypto) enter(symbol string, last float64, opportunity opportunity) {
	if last <= 0 {
		return
	}

	notional := crypto.slot(opportunity)

	if notional < config.System.MinCostEUR {
		return
	}

	feePct := crypto.takerFeePct(symbol)
	spreadBPS := crypto.quotes.spreadBPS(symbol)
	measurements := crypto.snapshot(symbol)
	quote := crypto.quotes.snapshot(symbol, last)
	quote = economics.StressQuote(quote, economics.AdverseSelectionBPS(measurements))

	if rejectErr := economics.ShouldReject(); rejectErr != nil {
		errnie.Error(rejectErr)

		return
	}

	buy := broker.Buy{
		Symbol:   symbol,
		Notional: notional,
		Quote:    quote,
		FeePct:   feePct,
	}

	fill, err := buy.FillPaper(crypto.wallet)

	if err != nil {
		errnie.Error(err)

		return
	}

	now := time.Now()
	playbook := primaryPlaybook(opportunity.Names)
	entryLabel := economics.EntryLabel(
		symbol, playbook, "buy", quote, notional, fill.Price, feePct, spreadBPS, now,
	)
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
		"why":                  triggerLabel(opportunity.Trigger),
		"conviction":           opportunity.Score,
		"edge":                 opportunity.Edge,
		"perspectives":         opportunity.Names,
		"playbook":             playbook,
		"slot_eur":             notional,
		"quote_age_ms":         entryLabel.QuoteAgeMS,
		"spread_bps":           entryLabel.SpreadBPS,
		"projected_slippage_bps": entryLabel.ProjectedSlippageBPS,
		"depth_coverage":       entryLabel.DepthCoverage,
		"playbook_econ_samples": econCount,
		"playbook_econ_mean":   econMean,
	})
	crypto.publishFill(fill)
	crypto.publishWallet()
}

func (crypto *Crypto) exit(
	symbol string,
	last float64,
	action perspectives.ActionType,
	reason string,
) {
	if last <= 0 {
		return
	}

	base := baseOf(symbol)
	binding, _ := crypto.wallet.PositionBindingFor(base)
	entry := crypto.wallet.AvgEntryFor(base)

	sell := broker.Sell{
		Symbol: symbol,
		Quote:  crypto.quotes.snapshot(symbol, last),
		FeePct: binding.TakerFeePct,
	}

	fill, err := sell.FillPaper(crypto.wallet)

	if err != nil {
		errnie.Error(err)

		return
	}

	if fill.Qty <= 0 {
		return
	}

	crypto.wallet.ClearPosition(base)
	crypto.positions.Close(symbol)
	crypto.open.Add(-1)
	crypto.tracker.Remove(symbol)

	realized := realizedReturn(entry, fill.Price)
	exitSpreadBPS := crypto.quotes.spreadBPS(symbol)
	exitLabel := economics.ExitLabel(
		symbol, binding.Playbook, entry, fill.Price, binding.TakerFeePct, exitSpreadBPS, time.Now(),
	)
	crypto.economics.RecordExit(exitLabel)
	crypto.publishAudit("exit", symbol, reason, map[string]any{
		"actual_return": realized,
		"net_return":    exitLabel.NetReturn,
		"success":       exitLabel.NetReturn > 0,
		"held_ms":       time.Since(binding.PredictedAt).Milliseconds(),
		"playbook":      binding.Playbook,
	})
	crypto.publishFill(fill)
	crypto.publishWallet()
}

func (crypto *Crypto) slot(opportunity opportunity) float64 {
	free := crypto.wallet.BalanceCopy()

	if free <= 0 || opportunity.Score <= 0 {
		return 0
	}

	share := crypto.opportunityShare(opportunity)

	if share <= 0 {
		return 0
	}

	return free * share
}

func primaryPlaybook(names []string) string {
	if len(names) == 0 {
		return ""
	}

	return names[0]
}

func triggerLabel(trigger perspectives.Measurement) string {
	return trigger.Source.String() + "." + trigger.Category.String()
}

func exitReason(action perspectives.ActionType) string {
	switch action {
	case perspectives.ActionStopLoss:
		return "thesis reversed — stop"
	case perspectives.ActionTakeProfit:
		return "thesis complete — take profit"
	case perspectives.ActionShort:
		return "thesis flipped — close long"
	default:
		return "thesis exit"
	}
}

func (crypto *Crypto) resolveEconomics(symbol string, last float64) {
	forwardLabels := crypto.economics.ResolveForward(symbol, last, time.Now())

	for _, label := range forwardLabels {
		crypto.publishAudit("forward", symbol, "forward return matured", map[string]any{
			"playbook":       label.Playbook,
			"forward_return": label.ForwardReturn,
			"net_return":     label.NetReturn,
			"round_trip_cost": label.RoundTripCostPct,
		})
	}
}

func realizedReturn(entry, exit float64) float64 {
	if entry <= 0 {
		return 0
	}

	return (exit - entry) / entry
}

func (crypto *Crypto) publishAudit(auditEvent, symbol, reason string, fields map[string]any) {
	if crypto.ui == nil {
		return
	}

	frame := map[string]any{
		"event":       "audit",
		"ts":          time.Now().UTC().Format(time.RFC3339Nano),
		"seq":         crypto.auditSeq.Add(1),
		"audit_event": auditEvent,
		"symbol":      symbol,
		"source":      "trader",
		"reason":      reason,
		"open":        crypto.open.Load(),
	}

	for key, value := range fields {
		frame[key] = value
	}

	crypto.ui.Send(&qpool.QValue[any]{Value: frame})
}

func (crypto *Crypto) publishFill(fill order.Fill) {
	if crypto.ui == nil {
		return
	}

	crypto.ui.Send(&qpool.QValue[any]{Value: fill})
}

func (crypto *Crypto) publishWallet() {
	if crypto.ui == nil {
		return
	}

	snapshot := crypto.wallet.Snapshot()

	crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":       "wallet",
		"ts":          time.Now().UTC().Format(time.RFC3339Nano),
		"Currency":    snapshot.Currency,
		"Balance":     snapshot.Balance,
		"ReservedEUR": snapshot.ReservedEUR,
		"FeePct":      snapshot.FeePct,
		"Inventory":   snapshot.Inventory,
		"AvgEntry":    snapshot.AvgEntry,
		"Marks":       snapshot.Marks,
	}})
}

func (crypto *Crypto) takerFeePct(symbol string) float64 {
	catalog := market.Catalog()

	if catalog != nil {
		return catalog.TakerFeePercent(symbol)
	}

	return crypto.wallet.FeePct
}

func lotDecimalsKnown(symbol string) bool {
	catalog := market.Catalog()

	if catalog == nil {
		return false
	}

	pair := catalog.Lookup(symbol)

	return pair != nil && pair.LotDecimals > 0
}

func lotDecimals(symbol string) int {
	catalog := market.Catalog()

	if catalog == nil {
		return 0
	}

	pair := catalog.Lookup(symbol)

	if pair == nil {
		return 0
	}

	return pair.LotDecimals
}

func baseOf(symbol string) string {
	if base, _, found := strings.Cut(symbol, "/"); found {
		return base
	}

	return symbol
}
