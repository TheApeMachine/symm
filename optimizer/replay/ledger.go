package replay

import (
	"sync"
	"time"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/market/perspectives"
)

var replayLedgerPool = sync.Pool{
	New: func() any {
		return newReplayLedger(DefaultReplayCosts())
	},
}

func acquireReplayLedger(costs ReplayCosts) *replayLedger {
	ledger := replayLedgerPool.Get().(*replayLedger)
	ledger.reset(costs)

	return ledger
}

func releaseReplayLedger(ledger *replayLedger) {
	replayLedgerPool.Put(ledger)
}

func halfSpreadSlippagePct(costs ReplayCosts, spreadBPS float64) float64 {
	if spreadBPS > 0 {
		return spreadBPS / 20000.0
	}

	return costs.SlippagePct
}

type replayPosition struct {
	entryPrice    float64
	quantity      float64
	cost          float64                 // total cash deployed (entry notional + entry fee)
	peak          float64                 // running max price since entry (drives trailing stops)
	entryAt       time.Time               // when the position opened (drives the elapsed subject + lifecycle)
	triggerType   perspectives.ActionType // ActionNone until an exit gate arms a resting protective order
	triggerOffset float64                 // resolved offset for the armed trigger
}

type pendingReplayAction struct {
	executeAt   time.Time
	executeTick int
	act         perspectives.Act
	measurement perspectives.Measurement
	snapshots   []perspectives.Measurement
}

type replayLedger struct {
	costs               ReplayCosts
	positions           map[string]replayPosition
	cash                float64
	reentryTickCooldown int
	ticksSinceClose     map[string]int
	realized            float64
	closedTrades        int
	fundBlocked         int
	observationScratch  map[perspectives.ObservationType]float64
	metricsScratch      map[string]float64
	reasonStates        map[string]*perspectives.ReasonState
	windowReason        perspectives.WindowReason
	snapshotScratch     []perspectives.Measurement
	pricePaths          map[string][]float64
	pending             []pendingReplayAction
	tickIndex           int
	medianInterval      time.Duration
	executionLatency    time.Duration
}

// effectiveCapital and effectiveFraction let any ReplayCosts — including the bare
// structs built in tests — drive a working account, falling back to the defaults
// when the wallet fields are unset.
func effectiveCapital(costs ReplayCosts) float64 {
	if costs.StartingCapital <= 0 {
		return DefaultStartingCapital
	}

	return costs.StartingCapital
}

func effectiveFraction(costs ReplayCosts) float64 {
	if costs.PositionFraction <= 0 || costs.PositionFraction > 1 {
		return DefaultPositionFraction
	}

	return costs.PositionFraction
}

func newReplayLedger(costs ReplayCosts) *replayLedger {
	return &replayLedger{
		costs:               costs,
		positions:           make(map[string]replayPosition),
		cash:                effectiveCapital(costs),
		reentryTickCooldown: 1,
		ticksSinceClose:     make(map[string]int),
		observationScratch:  make(map[perspectives.ObservationType]float64, 1),
		metricsScratch:      make(map[string]float64, 2),
		reasonStates:        make(map[string]*perspectives.ReasonState),
		pricePaths:          make(map[string][]float64),
	}
}

// reasonState returns the symbol's cross-tick reasoning memory, creating it on
// first use. One per symbol, threaded through EvaluateStateful each tick so the
// Thought tree's Then chains stay armed across the ticks of an episode.
func (ledger *replayLedger) reasonState(symbol string) *perspectives.ReasonState {
	state, ok := ledger.reasonStates[symbol]

	if !ok {
		state = perspectives.NewReasonState()
		ledger.reasonStates[symbol] = state
	}

	return state
}

func (ledger *replayLedger) reset(costs ReplayCosts) {
	ledger.costs = costs
	ledger.cash = effectiveCapital(costs)
	ledger.reentryTickCooldown = 1
	ledger.realized = 0
	ledger.closedTrades = 0
	ledger.fundBlocked = 0
	ledger.tickIndex = 0
	ledger.medianInterval = 0
	ledger.executionLatency = 0
	ledger.pending = ledger.pending[:0]

	clear(ledger.positions)
	clear(ledger.ticksSinceClose)
	clear(ledger.observationScratch)
	clear(ledger.metricsScratch)
	for _, state := range ledger.reasonStates {
		state.Reset()
	}

	clear(ledger.pricePaths)
}

func (ledger *replayLedger) configureExecutionStress(
	latency time.Duration,
	medianInterval time.Duration,
) {
	ledger.executionLatency = latency
	ledger.medianInterval = medianInterval
}

func (ledger *replayLedger) onTickStart(
	at time.Time,
	row perspectives.Measurement,
) {
	ledger.observePrice(row)
	ledger.flushPending(at, row)
	ledger.checkTriggers(row)
}

func (ledger *replayLedger) flushPending(
	at time.Time,
	currentRow perspectives.Measurement,
) {
	if len(ledger.pending) == 0 {
		return
	}

	remaining := ledger.pending[:0]

	for _, item := range ledger.pending {
		if !ledger.executionReady(item, at) {
			remaining = append(remaining, item)
			continue
		}

		fillRow := executionFillMeasurement(item.measurement, currentRow)

		// A resting maker entry that the price ran away from never fills — drop
		// it rather than applying a phantom fill.
		if makerEntryMissed(item.act.Type, item.measurement.Last, fillRow.Last) {
			continue
		}

		ledger.applyStressed(item.act, fillRow, item.snapshots)
	}

	ledger.pending = remaining
}

/*
makerEntryMissed reports whether a resting maker (limit/iceberg) buy entry failed
to fill because the price ran above the posted level during the execution-latency
window. Passive entries miss fast up-moves — the cost of not crossing the spread —
so modelling it removes the "a limit always fills" optimism that made limit
strictly dominate market in replay. Price reachability stands in for true queue
depletion, which the replay tape lacks the book depth to model.
*/
func makerEntryMissed(
	actionType perspectives.ActionType, postPrice, fillPrice float64,
) bool {
	if !perspectives.IsEntryAction(actionType) || !perspectives.IsMakerAction(actionType) {
		return false
	}

	if postPrice <= 0 || fillPrice <= 0 {
		return false
	}

	return fillPrice > postPrice
}

func executionFillMeasurement(
	signalRow perspectives.Measurement,
	currentRow perspectives.Measurement,
) perspectives.Measurement {
	if currentRow.Symbol != signalRow.Symbol || currentRow.Last <= 0 {
		return signalRow
	}

	fillRow := signalRow
	fillRow.Last = currentRow.Last
	fillRow.SpreadBPS = currentRow.SpreadBPS

	if !currentRow.At.IsZero() {
		fillRow.At = currentRow.At
	}

	return fillRow
}

func (ledger *replayLedger) executionReady(
	item pendingReplayAction,
	at time.Time,
) bool {
	if ledger.executionLatency <= 0 {
		return true
	}

	if !at.IsZero() && !item.executeAt.IsZero() {
		return !at.Before(item.executeAt)
	}

	return item.executeTick <= ledger.tickIndex
}

/*
positionState projects the ledger's open position for a symbol into the view the
Thought language reasons over (holding, entry, peak, current price, the clock).
*/
func (ledger *replayLedger) positionState(
	row perspectives.Measurement,
) perspectives.PositionState {
	position, open := ledger.positions[row.Symbol]

	if !open {
		return perspectives.PositionState{Holding: false, Last: row.Last, Now: row.At}
	}

	return perspectives.PositionState{
		Holding:    true,
		EntryPrice: position.entryPrice,
		Peak:       position.peak,
		Last:       row.Last,
		EntryAt:    position.entryAt,
		Now:        row.At,
	}
}

func (ledger *replayLedger) onTick(symbol string) {
	if symbol == "" || ledger.holding(symbol) {
		return
	}

	if ticksSinceClose, tracked := ledger.ticksSinceClose[symbol]; tracked {
		ledger.ticksSinceClose[symbol] = ticksSinceClose + 1
	}
}

func (ledger *replayLedger) apply(
	act perspectives.Act, measurement perspectives.Measurement,
) {
	ledger.applyStressed(act, measurement, nil)
}

func (ledger *replayLedger) queueAction(
	act perspectives.Act,
	measurement perspectives.Measurement,
	snapshots []perspectives.Measurement,
) {
	if ledger.executionLatency <= 0 {
		ledger.applyStressed(act, measurement, snapshots)

		return
	}

	latencyTicks := executionLatencyTicks(ledger.executionLatency, ledger.medianInterval)

	ledger.pending = append(ledger.pending, pendingReplayAction{
		executeAt:   measurement.At.Add(ledger.executionLatency),
		executeTick: ledger.tickIndex + latencyTicks,
		act:         act,
		measurement: measurement,
		snapshots:   snapshots,
	})
}

func (ledger *replayLedger) applyStressed(
	act perspectives.Act,
	measurement perspectives.Measurement,
	snapshots []perspectives.Measurement,
) {
	if measurement.Last <= 0 {
		return
	}

	slippagePct := executionSlippagePct(ledger.costs, measurement.SpreadBPS, snapshots)

	if perspectives.IsMakerAction(act.Type) && ledger.costs.ExecutionStressEnabled {
		slippagePct += broker.ReplayMakerAdverseSlippagePct(
			measurement.SpreadBPS,
			executionStressMultiplier(snapshots),
		)
	}

	feePct := ledger.costs.feePct(act.Type)

	switch act.Type {
	case perspectives.ActionLimit, perspectives.ActionMarket, perspectives.ActionIceberg:
		ledger.openLong(measurement.Symbol, measurement.Last, feePct, slippagePct, measurement.At)
	case perspectives.ActionSettlePosition:
		// Discretionary exit: cross the book now at the current price.
		ledger.closeLong(measurement.Symbol, measurement.Last, feePct, slippagePct)
	case perspectives.ActionStopLoss,
		perspectives.ActionStopLossLimit,
		perspectives.ActionTakeProfit,
		perspectives.ActionTakeProfitLimit,
		perspectives.ActionTrailingStop,
		perspectives.ActionTrailingStopLimit:
		// Protective exit: rest the order; it fills only when the price path
		// breaches its trigger (checked each tick in checkTriggers).
		ledger.armTrigger(measurement.Symbol, act)
	case perspectives.ActionNone:
		return
	}
}
