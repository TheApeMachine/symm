package replay

import (
	"math"
	"sort"
	"strings"
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

type replayMeasurements struct {
	global  map[perspectives.SourceType]perspectives.Measurement
	symbols map[string]map[perspectives.SourceType]perspectives.Measurement
}

func newReplayMeasurements() *replayMeasurements {
	return &replayMeasurements{
		global:  make(map[perspectives.SourceType]perspectives.Measurement),
		symbols: make(map[string]map[perspectives.SourceType]perspectives.Measurement),
	}
}

func (measurements *replayMeasurements) Add(
	measurement perspectives.Measurement,
) {
	if measurement.Symbol == "" {
		measurements.global[measurement.Source] = measurement

		return
	}

	symbolRows, ok := measurements.symbols[measurement.Symbol]

	if !ok {
		symbolRows = make(map[perspectives.SourceType]perspectives.Measurement)
		measurements.symbols[measurement.Symbol] = symbolRows
	}

	symbolRows[measurement.Source] = measurement
}

func (measurements *replayMeasurements) Snapshot(
	symbol string,
) []perspectives.Measurement {
	rows := make(
		[]perspectives.Measurement, 0,
		len(measurements.global)+len(measurements.symbols[symbol]),
	)

	rows = append(rows, sortedMeasurementsBySource(measurements.global)...)
	rows = append(rows, sortedMeasurementsBySource(measurements.symbols[symbol])...)

	return rows
}

func sortedMeasurementsBySource(
	rows map[perspectives.SourceType]perspectives.Measurement,
) []perspectives.Measurement {
	sorted := make([]perspectives.Measurement, 0, len(rows))

	for _, measurement := range rows {
		sorted = append(sorted, measurement)
	}

	sort.Slice(sorted, func(leftIndex, rightIndex int) bool {
		return sorted[leftIndex].Source < sorted[rightIndex].Source
	})

	return sorted
}

type replayPosition struct {
	entryPrice    float64
	quantity      float64
	cost          float64                 // total cash deployed (entry notional + entry fee)
	peak          float64                 // running max price since entry (drives trailing stops)
	entryAt       time.Time               // when the position opened (drives the elapsed subject + lifecycle)
	triggerType   perspectives.ActionType // ActionNone until an exit gate arms a resting protective order
	triggerOffset float64                 // per-node offset for the armed trigger (0 = use the global cost default)
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
	observationScratch  map[perspectives.ObservationType]float64
	metricsScratch      map[string]float64
	reasonStates        map[string]*perspectives.ReasonState
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
	ledger.tickIndex = 0
	ledger.medianInterval = 0
	ledger.executionLatency = 0
	ledger.pending = ledger.pending[:0]

	clear(ledger.positions)
	clear(ledger.ticksSinceClose)
	clear(ledger.observationScratch)
	clear(ledger.metricsScratch)
	clear(ledger.reasonStates)
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

/*
armTrigger attaches a resting protective exit to an open position. The most
recent exit gate wins, so a strategy can revise its protection (e.g. tighten a
stop) on later ticks. The act's Offset overrides the global trigger distance for
this position. No-ops when flat.
*/
func (ledger *replayLedger) armTrigger(
	symbol string, act perspectives.Act,
) {
	position, open := ledger.positions[symbol]

	if !open {
		return
	}

	position.triggerType = act.Type
	position.triggerOffset = act.Offset
	ledger.positions[symbol] = position
}

/*
checkTriggers advances the running peak and closes the position when the price
path breaches an armed protective trigger. Stops and trailing stops cross the
book on breach (eating any gap-through and slippage); the -limit variants rest
as maker orders and fill at their trigger level. Called once per tick.
*/
func (ledger *replayLedger) checkTriggers(row perspectives.Measurement) {
	if row.Symbol == "" || row.Last <= 0 {
		return
	}

	position, open := ledger.positions[row.Symbol]

	if !open || position.entryPrice <= 0 {
		return
	}

	if row.Last > position.peak {
		position.peak = row.Last
		ledger.positions[row.Symbol] = position
	}

	if position.triggerType == perspectives.ActionNone {
		return
	}

	level, breached := triggerLevel(ledger.costs, position, row.Last)

	if !breached {
		return
	}

	ledger.closeAtTrigger(row.Symbol, position.triggerType, level, row.Last, row.SpreadBPS)
}

/*
triggerLevel returns the protective trigger price for the armed exit and whether
the current price has breached it. Long positions only.
*/
func triggerLevel(
	costs ReplayCosts, position replayPosition, price float64,
) (level float64, breached bool) {
	switch position.triggerType {
	case perspectives.ActionStopLoss, perspectives.ActionStopLossLimit:
		level = position.entryPrice * (1 - triggerOffset(position.triggerOffset, costs.StopLossPct))

		return level, price <= level
	case perspectives.ActionTakeProfit, perspectives.ActionTakeProfitLimit:
		level = position.entryPrice * (1 + triggerOffset(position.triggerOffset, costs.TakeProfitPct))

		return level, price >= level
	case perspectives.ActionTrailingStop, perspectives.ActionTrailingStopLimit:
		level = position.peak * (1 - triggerOffset(position.triggerOffset, costs.TrailingPct))

		return level, price <= level
	default:
		return 0, false
	}
}

// triggerOffset prefers the per-node offset the playbook armed, falling back to
// the global cost default when the node did not specify one or specified a
// nonsensical fraction (<=0 or >=1 — e.g. a stop "below" entry by 150% would arm
// above entry). The optimizer will generate offsets, so this clamps bad ones.
func triggerOffset(perNode, global float64) float64 {
	if perNode > 0 && perNode < 1 {
		return perNode
	}

	return global
}

func exitRestsAsLimit(actionType perspectives.ActionType) bool {
	switch actionType {
	case perspectives.ActionStopLossLimit,
		perspectives.ActionTakeProfitLimit,
		perspectives.ActionTrailingStopLimit:
		return true
	default:
		return false
	}
}

/*
closeAtTrigger realizes a protective exit. Market-style triggers (stop, take,
trailing) cross the book: a stop or trail fills at the worse of the trigger and
the current price (gap-through), paying taker fee and slippage. The -limit
variants rest as maker orders and fill at their trigger level with the maker fee
and no crossing slippage.
*/
func (ledger *replayLedger) closeAtTrigger(
	symbol string,
	actionType perspectives.ActionType,
	level float64,
	price float64,
	spreadBPS float64,
) {
	position, open := ledger.positions[symbol]

	if !open || position.entryPrice <= 0 {
		return
	}

	var exitFill, feePct float64

	if exitRestsAsLimit(actionType) {
		exitFill = level
		feePct = ledger.costs.MakerFeePct
	} else {
		fill := level

		// A market stop/trail eats a downside gap-through; a market take-profit
		// does not assume a favourable upside gap.
		if actionType != perspectives.ActionTakeProfit && price < level {
			fill = price
		}

		exitFill = fill * (1 - halfSpreadSlippagePct(ledger.costs, spreadBPS))
		feePct = ledger.costs.TakerFeePct
	}

	ledger.settle(symbol, exitFill, feePct)
}

/*
openLong sizes an entry from available account cash. It is the funding gate that
makes the replay match a real €200 account: an entry is taken only when the
wallet holds the pair's quote currency and has cash free, and it deploys
PositionFraction of that cash. A strategy that wants to be in many positions at
once therefore only books the trades it could actually pay for — the rest are
skipped exactly as live rejects them for insufficient funds.
*/
func (ledger *replayLedger) openLong(
	symbol string,
	price float64,
	feePct float64,
	slippagePct float64,
	at time.Time,
) {
	if _, open := ledger.positions[symbol]; open {
		return
	}

	if ticksSinceClose, tracked := ledger.ticksSinceClose[symbol]; tracked &&
		ticksSinceClose < ledger.reentryTickCooldown {
		return
	}

	if !ledger.fundableSymbol(symbol) {
		return
	}

	spendable := effectiveFraction(ledger.costs) * ledger.cash

	if spendable <= 0 {
		return
	}

	entryFill := price * (1 + slippagePct)

	if entryFill <= 0 {
		return
	}

	// spendable buys quantity units and pays the entry fee on that notional:
	// quantity*entryFill*(1+feePct) == spendable.
	quantity := spendable / (entryFill * (1 + feePct))

	if quantity <= 0 {
		return
	}

	ledger.cash -= spendable

	ledger.positions[symbol] = replayPosition{
		entryPrice:  entryFill,
		quantity:    quantity,
		cost:        spendable,
		peak:        entryFill,
		entryAt:     at,
		triggerType: perspectives.ActionNone,
	}

	delete(ledger.ticksSinceClose, symbol)
}

/*
fundableSymbol reports whether the wallet can pay for the pair: a buy spends the
pair's quote currency, so only pairs quoted in WalletCurrency are tradeable. An
empty WalletCurrency or an unparseable symbol falls back to fundable so plain
single-symbol replays are unaffected.
*/
func (ledger *replayLedger) fundableSymbol(symbol string) bool {
	if ledger.costs.WalletCurrency == "" {
		return true
	}

	slash := strings.LastIndex(symbol, "/")

	if slash < 0 {
		return true
	}

	return symbol[slash+1:] == ledger.costs.WalletCurrency
}

/*
settle closes a position back to cash and books the realized P&L in account
currency. exitFill is the per-unit fill price and feePct the exit fee fraction.
*/
func (ledger *replayLedger) settle(symbol string, exitFill, feePct float64) {
	position, open := ledger.positions[symbol]

	if !open {
		return
	}

	netProceeds := position.quantity * exitFill * (1 - feePct)

	ledger.cash += netProceeds
	ledger.realized += netProceeds - position.cost
	ledger.closedTrades++
	delete(ledger.positions, symbol)
	ledger.ticksSinceClose[symbol] = 0
}

func (ledger *replayLedger) previewClosePnL(
	symbol string,
	price float64,
	spreadBPS float64,
) float64 {
	position, open := ledger.positions[symbol]

	if !open || position.entryPrice <= 0 || price <= 0 {
		return 0
	}

	slippagePct := halfSpreadSlippagePct(ledger.costs, spreadBPS)
	exitFill := price * (1 - slippagePct)

	return (exitFill - position.entryPrice) / position.entryPrice
}

func (ledger *replayLedger) closeLong(
	symbol string,
	price float64,
	feePct float64,
	slippagePct float64,
) {
	position, open := ledger.positions[symbol]

	if !open || position.entryPrice <= 0 {
		return
	}

	exitFill := price * (1 - slippagePct)

	ledger.settle(symbol, exitFill, feePct)
}

func (ledger *replayLedger) observations(
	symbol string,
) map[perspectives.ObservationType]float64 {
	clear(ledger.observationScratch)

	if ledger.holding(symbol) {
		ledger.observationScratch[perspectives.ObservationHolding] = 1

		return ledger.observationScratch
	}

	ledger.observationScratch[perspectives.ObservationNotHolding] = 1

	return ledger.observationScratch
}

func (ledger *replayLedger) metrics(
	measurement perspectives.Measurement,
) map[string]float64 {
	clear(ledger.metricsScratch)
	ledger.metricsScratch["last"] = measurement.Last

	position, open := ledger.positions[measurement.Symbol]

	if !open || position.entryPrice <= 0 || measurement.Last <= 0 {
		return ledger.metricsScratch
	}

	slippagePct := halfSpreadSlippagePct(ledger.costs, measurement.SpreadBPS)
	exitFill := measurement.Last * (1 - slippagePct)
	change := exitFill - position.entryPrice
	ledger.metricsScratch["unrealized_return"] = (change / position.entryPrice) * 100

	return ledger.metricsScratch
}

func (ledger *replayLedger) holding(symbol string) bool {
	_, open := ledger.positions[symbol]

	return open
}

/*
realizedReturn is the account's realized P&L as a fraction of starting capital,
so the score is a return on the real €200 — not a capital-free sum of per-trade
percentages that silently assumed unlimited money.
*/
func (ledger *replayLedger) realizedReturn() float64 {
	return ledger.realized / effectiveCapital(ledger.costs)
}

func HoldoutDecay(trainPerTrade float64, testPerTrade float64) float64 {
	return holdoutDecay(trainPerTrade, testPerTrade)
}

func holdoutDecay(trainPerTrade float64, testPerTrade float64) float64 {
	if trainPerTrade <= 0 {
		return math.Inf(1)
	}

	return (trainPerTrade - testPerTrade) / trainPerTrade
}
