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
	"github.com/theapemachine/symm/kraken/order"
	decision "github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/wallet"
)

/*
Crypto is the trade desk. It is deliberately thin: the decision of whether to enter,
hold, or leave a position is not made here — it is made by the perspectives system,
which reads the live measurement set for a symbol and returns a verdict. The desk's
only jobs are to keep each symbol's latest readings, ask the perspectives for the
verdict, and turn that verdict into a paper order sized by the wallet's own policy.

Entry and exit are the same thesis re-evaluated, not two strategies: a flat symbol is
offered to the playbooks for an entry verdict, a held one is offered the identical
playbooks with ObservationHolding for an exit verdict. Every number the desk uses —
slot size, the minimum tradeable cost, the prediction horizon — comes from
config.System, the single home for policy; the desk invents none of its own.
*/
type Crypto struct {
	ctx          context.Context
	cancel       context.CancelFunc
	pool         *qpool.Q
	measurements *qpool.Subscriber
	ui           *qpool.BroadcastGroup
	wallet       *wallet.Wallet
	tracker      *focus.Set
	mu           sync.RWMutex
	readings     map[string]map[perspectives.CategoryType]perspectives.Measurement
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
		ctx:      ctx,
		cancel:   cancel,
		pool:     pool,
		wallet:   tradingWallet,
		tracker:  tracker,
		readings: make(map[string]map[perspectives.CategoryType]perspectives.Measurement),
	}

	group := pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	crypto.measurements = group.Subscribe("trader:measurements", 128)
	crypto.ui = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

	return crypto
}

/*
Tick consumes the measurement bus directly: every reading is recorded and the
affected symbol re-evaluated. The price the desk needs to size and fill rides in on
the measurement itself, so there is no separate market feed. The wallet snapshot is
republished on the configured heartbeat so a dashboard that connects late still sees
the balance.
*/
func (crypto *Crypto) Tick() error {
	heartbeat := time.NewTicker(config.System.UIHeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-crypto.ctx.Done():
			return crypto.ctx.Err()
		case <-heartbeat.C:
			crypto.publishWallet()
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

// ResendWallet publishes the current wallet snapshot so a freshly connected
// dashboard shows the opening balance before any trade happens.
func (crypto *Crypto) ResendWallet() {
	crypto.publishWallet()
}

func (crypto *Crypto) record(measurement perspectives.Measurement) {
	crypto.mu.Lock()
	defer crypto.mu.Unlock()

	set := crypto.readings[measurement.Symbol]

	if set == nil {
		set = make(map[perspectives.CategoryType]perspectives.Measurement)
		crypto.readings[measurement.Symbol] = set
	}

	set[measurement.Category] = measurement
}

// evaluate routes a symbol to the entry or the exit view of its perspective: a held
// symbol is offered the playbooks with ObservationHolding for an exit verdict, a flat
// one is offered them for an entry verdict.
func (crypto *Crypto) evaluate(symbol string, last float64) {
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

	set := crypto.readings[symbol]
	measurements := make([]perspectives.Measurement, 0, len(set))

	for _, measurement := range set {
		measurements = append(measurements, measurement)
	}

	return measurements
}

// consider asks the playbooks for an entry verdict on a flat symbol and enters when
// one authorizes it.
func (crypto *Crypto) consider(symbol string, last float64, measurements []perspectives.Measurement) {
	action, _ := decision.Decide(measurements, nil)

	if action == nil || *action != perspectives.ActionEnter {
		return
	}

	crypto.enter(symbol, last, strongest(measurements))
}

// manage asks the same playbooks for the exit verdict on a held symbol, walking them
// with ObservationHolding. A stop, a take-profit, or a flip all close the position —
// the reason the trade was opened deciding when it is closed.
func (crypto *Crypto) manage(symbol string, last float64, measurements []perspectives.Measurement) {
	action, _ := decision.Decide(measurements, []perspectives.ObservationType{perspectives.ObservationHolding})

	if action == nil {
		return
	}

	switch *action {
	case perspectives.ActionStopLoss, perspectives.ActionTakeProfit, perspectives.ActionShort:
		crypto.exit(symbol, last, *action)
	}
}

// enter opens a paper position sized from live free balance and the configured
// per-slot cap. As positions open the free balance falls and the next slot shrinks,
// so concurrent exposure is governed by capital and config.MinCostEUR — never an
// arbitrary count the desk picks.
func (crypto *Crypto) enter(symbol string, last float64, trigger perspectives.Measurement) {
	if last <= 0 {
		return
	}

	notional := crypto.slot()

	if notional < config.System.MinCostEUR {
		return
	}

	buy := broker.Buy{
		Symbol:   symbol,
		Notional: notional,
		Quote:    broker.Quote{Last: last, At: time.Now()},
	}

	fill, err := buy.FillPaper(crypto.wallet)

	if err != nil {
		errnie.Error(err)

		return
	}

	now := time.Now()
	crypto.wallet.BindPosition(baseOf(symbol), wallet.PositionBinding{
		Source:      "perspective",
		PredictedAt: now,
		DueAt:       now.Add(config.System.PerspectiveTTL),
	})
	crypto.open.Add(1)
	crypto.tracker.Add(symbol)

	crypto.publishAudit("entry", symbol, "perspective entry on "+triggerLabel(trigger), map[string]any{
		"why":        triggerLabel(trigger),
		"conviction": trigger.SNR,
		"slot_eur":   notional,
	})
	crypto.publishFill(fill)
	crypto.publishWallet()
}

// exit closes the full position at the live price and settles the wallet.
func (crypto *Crypto) exit(symbol string, last float64, action perspectives.ActionType) {
	if last <= 0 {
		return
	}

	base := baseOf(symbol)
	binding, _ := crypto.wallet.PositionBindingFor(base)
	entry := crypto.wallet.AvgEntryFor(base)

	sell := broker.Sell{
		Symbol: symbol,
		Quote:  broker.Quote{Last: last, At: time.Now()},
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
	crypto.open.Add(-1)
	crypto.tracker.Remove(symbol)

	realized := realizedReturn(entry, fill.Price)
	crypto.publishAudit("exit", symbol, exitReason(action), map[string]any{
		"actual_return": realized,
		"success":       realized > 0,
		"held_ms":       time.Since(binding.PredictedAt).Milliseconds(),
	})
	crypto.publishFill(fill)
	crypto.publishWallet()
}

// slot sizes one entry as the configured fraction of live free balance.
func (crypto *Crypto) slot() float64 {
	free := crypto.wallet.BalanceCopy()

	if free <= 0 {
		return 0
	}

	return free * config.System.MaxSlotPct / 100
}

// strongest returns the highest-SNR reading in the set: the measurement that most
// clears its own noise floor, taken as the trigger that authorized the entry.
func strongest(measurements []perspectives.Measurement) perspectives.Measurement {
	var best perspectives.Measurement

	for _, measurement := range measurements {
		if measurement.SNR > best.SNR {
			best = measurement
		}
	}

	return best
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

// realizedReturn is the fractional move from entry to exit price.
func realizedReturn(entry, exit float64) float64 {
	if entry <= 0 {
		return 0
	}

	return (exit - entry) / entry
}

// publishAudit narrates a desk decision to the dashboard's audit log.
func (crypto *Crypto) publishAudit(auditEvent, symbol, reason string, fields map[string]any) {
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
	crypto.ui.Send(&qpool.QValue[any]{Value: fill})
}

func (crypto *Crypto) publishWallet() {
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

// baseOf returns the base currency of a "BASE/QUOTE" pair symbol.
func baseOf(symbol string) string {
	if base, _, found := strings.Cut(symbol, "/"); found {
		return base
	}

	return symbol
}
