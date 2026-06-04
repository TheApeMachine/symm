package replay

import (
	"math"
	"sort"
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
	entryPrice  float64
	quantity    float64
	peak        float64                 // running max price since entry (drives trailing stops)
	triggerType perspectives.ActionType // ActionNone until an exit gate arms a resting protective order
}

type pendingReplayAction struct {
	executeAt   time.Time
	executeTick int
	action      perspectives.ActionType
	measurement perspectives.Measurement
	snapshots   []perspectives.Measurement
}

type replayLedger struct {
	costs               ReplayCosts
	positions           map[string]replayPosition
	reentryTickCooldown int
	ticksSinceClose     map[string]int
	realized            float64
	closedTrades        int
	observationScratch  map[perspectives.ObservationType]float64
	metricsScratch      map[string]float64
	pending             []pendingReplayAction
	tickIndex           int
	medianInterval      time.Duration
	executionLatency    time.Duration
}

func newReplayLedger(costs ReplayCosts) *replayLedger {
	return &replayLedger{
		costs:               costs,
		positions:           make(map[string]replayPosition),
		reentryTickCooldown: 1,
		ticksSinceClose:     make(map[string]int),
		observationScratch:  make(map[perspectives.ObservationType]float64, 1),
		metricsScratch:      make(map[string]float64, 2),
	}
}

func (ledger *replayLedger) reset(costs ReplayCosts) {
	ledger.costs = costs
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
		if makerEntryMissed(item.action, item.measurement.Last, fillRow.Last) {
			continue
		}

		ledger.applyStressed(item.action, fillRow, item.snapshots)
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

func (ledger *replayLedger) onTick(symbol string) {
	if symbol == "" || ledger.holding(symbol) {
		return
	}

	if ticksSinceClose, tracked := ledger.ticksSinceClose[symbol]; tracked {
		ledger.ticksSinceClose[symbol] = ticksSinceClose + 1
	}
}

func (ledger *replayLedger) apply(
	actionType perspectives.ActionType, measurement perspectives.Measurement,
) {
	ledger.applyStressed(actionType, measurement, nil)
}

func (ledger *replayLedger) queueAction(
	actionType perspectives.ActionType,
	measurement perspectives.Measurement,
	snapshots []perspectives.Measurement,
) {
	if ledger.executionLatency <= 0 {
		ledger.applyStressed(actionType, measurement, snapshots)

		return
	}

	latencyTicks := executionLatencyTicks(ledger.executionLatency, ledger.medianInterval)

	ledger.pending = append(ledger.pending, pendingReplayAction{
		executeAt:   measurement.At.Add(ledger.executionLatency),
		executeTick: ledger.tickIndex + latencyTicks,
		action:      actionType,
		measurement: measurement,
		snapshots:   snapshots,
	})
}

func (ledger *replayLedger) applyStressed(
	actionType perspectives.ActionType,
	measurement perspectives.Measurement,
	snapshots []perspectives.Measurement,
) {
	if measurement.Last <= 0 {
		return
	}

	slippagePct := executionSlippagePct(ledger.costs, measurement.SpreadBPS, snapshots)

	if perspectives.IsMakerAction(actionType) && ledger.costs.ExecutionStressEnabled {
		slippagePct += broker.ReplayMakerAdverseSlippagePct(
			measurement.SpreadBPS,
			executionStressMultiplier(snapshots),
		)
	}

	feePct := ledger.costs.feePct(actionType)

	switch actionType {
	case perspectives.ActionLimit, perspectives.ActionMarket, perspectives.ActionIceberg:
		ledger.openLong(measurement.Symbol, measurement.Last, feePct, slippagePct)
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
		ledger.armTrigger(measurement.Symbol, actionType)
	case perspectives.ActionNone:
		return
	}
}

/*
armTrigger attaches a resting protective exit to an open position. The most
recent exit gate wins, so a strategy can revise its protection (e.g. tighten a
stop) on later ticks. No-ops when flat.
*/
func (ledger *replayLedger) armTrigger(
	symbol string, actionType perspectives.ActionType,
) {
	position, open := ledger.positions[symbol]

	if !open {
		return
	}

	position.triggerType = actionType
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
		level = position.entryPrice * (1 - costs.StopLossPct)

		return level, price <= level
	case perspectives.ActionTakeProfit, perspectives.ActionTakeProfitLimit:
		level = position.entryPrice * (1 + costs.TakeProfitPct)

		return level, price >= level
	case perspectives.ActionTrailingStop, perspectives.ActionTrailingStopLimit:
		level = position.peak * (1 - costs.TrailingPct)

		return level, price <= level
	default:
		return 0, false
	}
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

	gross := (exitFill - position.entryPrice) / position.entryPrice

	ledger.realized += gross - feePct
	ledger.closedTrades++
	delete(ledger.positions, symbol)
	ledger.ticksSinceClose[symbol] = 0
}

func (ledger *replayLedger) openLong(
	symbol string,
	price float64,
	feePct float64,
	slippagePct float64,
) {
	if _, open := ledger.positions[symbol]; open {
		return
	}

	if ticksSinceClose, tracked := ledger.ticksSinceClose[symbol]; tracked &&
		ticksSinceClose < ledger.reentryTickCooldown {
		return
	}

	entryFill := price * (1 + slippagePct)

	ledger.positions[symbol] = replayPosition{
		entryPrice:  entryFill,
		quantity:    1,
		peak:        entryFill,
		triggerType: perspectives.ActionNone,
	}

	ledger.realized -= feePct
	delete(ledger.ticksSinceClose, symbol)
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
	gross := (exitFill - position.entryPrice) / position.entryPrice

	ledger.realized += gross - feePct
	ledger.closedTrades++
	delete(ledger.positions, symbol)
	ledger.ticksSinceClose[symbol] = 0
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

func (ledger *replayLedger) realizedReturn() float64 {
	return ledger.realized
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
