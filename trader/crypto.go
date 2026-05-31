package trader

import (
	"context"
	"fmt"
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
	ctx                     context.Context
	cancel                  context.CancelFunc
	pool                    *qpool.Q
	measurements            *qpool.Subscriber
	ui                      *qpool.BroadcastGroup
	wallet                  *wallet.Wallet
	tracker                 *focus.Set
	story                   *decision.Story
	positions               *positionBook
	mu                      sync.RWMutex
	readings                map[string]map[perspectives.SourceType]timedMeasurement
	quotes                  *quoteCache
	economics               *economics.Desk
	paper                   *paperSession
	live                    *liveSession
	makers                  *makerDesk
	makerFillMu             sync.Mutex
	open                    atomic.Int64
	auditSeq                atomic.Uint64
	pulseSeq                atomic.Uint64
	crossSection            atomic.Pointer[crossSectionSnapshot]
	priorPulseMultiple      float64
	priorPulseMultipleValid bool
	runtime                 *config.ScopedRuntime
	decisionTraceMu         sync.Mutex
	decisionTraceRows       []decisionTraceRow
	auditLog                *AuditLog
}

func NewCrypto(
	ctx context.Context,
	pool *qpool.Q,
	tradingWallet *wallet.Wallet,
	tracker *focus.Set,
	runtime *config.ScopedRuntime,
) (*Crypto, error) {
	ctx, cancel := context.WithCancel(ctx)

	if runtime == nil {
		runtime = config.Runtime
	}

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
		runtime:   runtime,
	}

	group := pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	crypto.measurements = group.Subscribe("trader:measurements", 128)
	crypto.ui = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)
	crypto.paper = NewPaperSession(ctx)
	crypto.makers = newMakerDesk()

	if liveEnabled(tradingWallet) {
		session, sessionErr := NewLiveSession(ctx, config.System.KrakenAPIKey, config.System.KrakenAPISecret)

		if sessionErr != nil {
			crypto.cancel()

			return nil, fmt.Errorf("live session: %w", sessionErr)
		}

		crypto.live = session
	}

	if auditPath := strings.TrimSpace(config.System.AuditFile); auditPath != "" {
		auditLog, auditErr := OpenAuditLog(
			auditPath,
			config.System.AuditMaxFileBytes,
			config.System.AuditMaxFiles,
			config.System.AuditGateRejectCooldown,
		)

		if auditErr != nil {
			errnie.Error(auditErr)
		} else {
			crypto.auditLog = auditLog
		}
	}

	market.SetBookHealthSink(&bookHealthSink{crypto: crypto})

	return crypto, nil
}

func (crypto *Crypto) Close() error {
	crypto.cancel()

	if crypto.paper != nil {
		if err := crypto.paper.Close(); err != nil {
			return err
		}
	}

	if crypto.live != nil {
		if err := crypto.live.Close(); err != nil {
			return err
		}
	}

	if crypto.auditLog != nil {
		if err := crypto.auditLog.Close(); err != nil {
			return err
		}
	}

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
	if crypto.hasPendingEntry(symbol) {
		return
	}

	entryContext := crypto.entryContextProvider(symbol, measurements)
	verdicts := decision.EntryVerdictsWithContext(measurements, nil, entryContext)
	defer decision.ReleaseEntryVerdicts(verdicts)

	crypto.story.RecordEntryVerdicts(symbol, verdicts)
	crypto.recordEntryVerdicts(symbol, measurements, verdicts)

	entryDecisions := decision.DecisionsWithContext(measurements, nil, entryContext)
	crypto.story.RecordEntry(symbol, entryDecisions)

	opportunity, ok := crypto.entryOpportunity(symbol, measurements, entryContext)

	if !ok {
		if reason, fields := crypto.entryRejectReason(symbol, measurements, entryContext); reason != "" {
			crypto.publishEntryReject(symbol, reason, fields)
		}

		return
	}

	opportunity, ok = crypto.calibrateOpportunity(opportunity)

	if !ok {
		crypto.publishEntryReject(symbol, "edge_below_baseline", crypto.calibrateRejectFields(opportunity))

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
	softAllowed := time.Since(binding.PredictedAt) >= crypto.scopedRuntime().Risk.MinExhaustHold
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

	if notional < crypto.scopedRuntime().Risk.MinCostEUR {
		return
	}

	feePct := crypto.takerFeePct(symbol)
	spreadBPS := crypto.quotes.spreadBPS(symbol)
	measurements := crypto.snapshot(symbol)
	stressRegime := broker.StressRegimeFrom(measurements)
	quote := crypto.prepareEntryQuote(symbol, last, measurements)
	playbook := primaryPlaybook(opportunity.Names)

	if crypto.scopedRuntime().Execution.UseMakerEntries {
		if err := crypto.submitMakerEntry(
			symbol, notional, quote, opportunity, playbook, spreadBPS, crypto.makerFeePct(symbol), stressRegime,
		); err != nil {
			errnie.Error(err)
		}

		return
	}

	buy := broker.Buy{
		Symbol:       symbol,
		Notional:     notional,
		Quote:        quote,
		FeePct:       feePct,
		Execution:    crypto.scopedRuntime().Execution,
		StressRegime: stressRegime,
	}

	if err := crypto.submitEntry(buy, opportunity, playbook, spreadBPS); err != nil {
		errnie.Error(err)
	}
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
	binding, held := crypto.wallet.PositionBindingFor(base)

	if !held {
		return
	}

	if crypto.hasPendingExit(symbol) {
		return
	}

	if crypto.wallet.InventoryQty(base) <= crypto.scopedRuntime().Execution.LiveInventoryEpsilon {
		return
	}

	entry := crypto.wallet.AvgEntryFor(base)
	spreadBPS := crypto.quotes.spreadBPS(symbol)

	sell := broker.Sell{
		Symbol:       symbol,
		Quote:        crypto.quotes.snapshot(symbol, last),
		FeePct:       binding.TakerFeePct,
		Execution:    crypto.scopedRuntime().Execution,
		StressRegime: broker.StressRegimeFrom(crypto.snapshot(symbol)),
	}

	if err := crypto.submitExit(sell, binding, entry, spreadBPS, reason); err != nil {
		errnie.Error(err)
	}
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

func (crypto *Crypto) scopedRuntime() *config.ScopedRuntime {
	if crypto.runtime != nil {
		return crypto.runtime
	}

	if config.Runtime != nil {
		return config.Runtime
	}

	return config.NewRuntime(config.System)
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
	now := time.Now()
	forwardLabels := crypto.economics.ResolveForward(symbol, last, now)

	for _, label := range forwardLabels {
		crypto.publishAudit("forward", symbol, "forward return matured", map[string]any{
			"playbook":        label.Playbook,
			"forward_return":  label.ForwardReturn,
			"net_return":      label.NetReturn,
			"round_trip_cost": label.RoundTripCostPct,
		})
	}

	crypto.economics.ResolveGateReject(symbol, last, now)
}

func realizedReturn(entry, exit float64) float64 {
	if entry <= 0 {
		return 0
	}

	return (exit - entry) / entry
}

func (crypto *Crypto) publishAudit(auditEvent, symbol, reason string, fields map[string]any) {
	if crypto.ui == nil && crypto.auditLog == nil {
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

	if crypto.auditLog != nil {
		if err := crypto.auditLog.Append(auditEvent, frame); err != nil {
			errnie.Error(err)
		}
	}

	if crypto.ui == nil {
		return
	}

	if auditEvent != "entry" &&
		auditEvent != "exit" &&
		auditEvent != "book_diverged" &&
		auditEvent != "book_recovered" {
		return
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
