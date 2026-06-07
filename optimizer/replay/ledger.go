package replay

import (
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/execution"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
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
	side          trading.Side
	entryPrice    float64
	quantity      float64
	cost          float64 // total cash deployed (entry notional + entry fee)
	peak          float64 // running max price since entry (long trailing stops)
	trough        float64 // running min price since entry (short trailing stops)
	entryAt       time.Time
	triggerType   reasoning.ActionType
	triggerOffset float64
}

type pendingReplayAction struct {
	executeAt   time.Time
	executeTick int
	act         reasoning.Act
	measurement types.Measurement
	snapshots   []types.Measurement
}

type replayLedger struct {
	costs               ReplayCosts
	positions           map[string]replayPosition
	cash                map[string]float64
	reentryTickCooldown int
	ticksSinceClose     map[string]int
	realized            float64
	closedTrades        int
	fundBlocked         int
	preflightBlocked    int
	instrumentRules     *broker.InstrumentRulesCache
	exposureTicks       int
	observationScratch  map[types.ObservationType]float64
	metricsScratch      map[string]float64
	reasonStates        map[string]*reasoning.ReasonState
	windowReason        reasoning.WindowReason
	snapshotScratch     []types.Measurement
	pricePaths          map[string][]float64
	pending             []pendingReplayAction
	pendingMakers       []pendingMakerEntry
	entryBatch          []batchedReplayEntry
	entryBatchDeadline  time.Time
	entryConviction     map[string]float64
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
		instrumentRules:     costs.InstrumentRules,
		positions:           make(map[string]replayPosition),
		cash:                cloneWalletBalances(costs),
		reentryTickCooldown: 1,
		ticksSinceClose:     make(map[string]int),
		observationScratch:  make(map[types.ObservationType]float64, 1),
		metricsScratch:      make(map[string]float64, 2),
		reasonStates:        make(map[string]*reasoning.ReasonState),
		pricePaths:          make(map[string][]float64),
		entryConviction:     make(map[string]float64),
	}
}

func cloneWalletBalances(costs ReplayCosts) map[string]float64 {
	cash := make(map[string]float64, len(costs.WalletBalances))

	for currency, balance := range costs.WalletBalances {
		cash[currency] = balance
	}

	if len(cash) == 0 && costs.WalletCurrency != "" {
		cash[costs.WalletCurrency] = effectiveCapital(costs)
	}

	return cash
}

// reasonState returns the symbol's cross-tick reasoning memory, creating it on
// first use. One per symbol, threaded through EvaluateStateful each tick so the
// Thought tree's Then chains stay armed across the ticks of an episode.
func (ledger *replayLedger) reasonState(symbol string) *reasoning.ReasonState {
	state, ok := ledger.reasonStates[symbol]

	if !ok {
		state = reasoning.NewReasonState()
		ledger.reasonStates[symbol] = state
	}

	return state
}

func (ledger *replayLedger) reset(costs ReplayCosts) {
	ledger.costs = costs
	ledger.cash = cloneWalletBalances(costs)
	ledger.reentryTickCooldown = 1
	ledger.realized = 0
	ledger.closedTrades = 0
	ledger.fundBlocked = 0
	ledger.preflightBlocked = 0
	ledger.instrumentRules = costs.InstrumentRules
	ledger.exposureTicks = 0
	ledger.tickIndex = 0
	ledger.medianInterval = 0
	ledger.executionLatency = 0
	ledger.pending = ledger.pending[:0]
	ledger.pendingMakers = ledger.pendingMakers[:0]
	ledger.entryBatch = ledger.entryBatch[:0]
	ledger.entryBatchDeadline = time.Time{}
	clear(ledger.entryConviction)

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
	row types.Measurement,
) {
	ledger.observePrice(row)
	ledger.advanceMakerQueues(row)
	ledger.flushEntryBatch(at)
	ledger.flushPending(at, row)
	ledger.checkTriggers(row)
}

func (ledger *replayLedger) flushPending(
	at time.Time,
	currentRow types.Measurement,
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
		if makerEntryMissed(item.act.Type, item.act.Side, item.measurement.Last, fillRow.Last) {
			continue
		}

		ledger.applyStressed(item.act, fillRow, item.snapshots, 1)
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
	actionType reasoning.ActionType,
	side trading.Side,
	postPrice, fillPrice float64,
) bool {
	if !reasoning.IsEntryAction(actionType) || !reasoning.IsMakerAction(actionType) {
		return false
	}

	if postPrice <= 0 || fillPrice <= 0 {
		return false
	}

	if side == trading.Sell {
		return fillPrice < postPrice
	}

	return fillPrice > postPrice
}

func executionFillMeasurement(
	signalRow types.Measurement,
	currentRow types.Measurement,
) types.Measurement {
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
	row types.Measurement,
) reasoning.PositionState {
	position, open := ledger.positions[row.Symbol]

	if !open {
		return reasoning.PositionState{Holding: false, Last: row.Last, Now: row.At}
	}

	return reasoning.PositionState{
		Holding:    true,
		Side:       position.side,
		EntryPrice: position.entryPrice,
		Peak:       position.peak,
		Trough:     position.trough,
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

func (ledger *replayLedger) queueAction(
	act reasoning.Act,
	measurement types.Measurement,
	snapshots []types.Measurement,
) {
	if reasoning.IsEntryAction(act.Type) {
		ledger.queueEntryAction(act, measurement, snapshots)

		return
	}

	ledger.queueActionImmediate(act, measurement, snapshots)
}

func (ledger *replayLedger) queueActionImmediate(
	act reasoning.Act,
	measurement types.Measurement,
	snapshots []types.Measurement,
) {
	if ledger.executionLatency <= 0 {
		ledger.applyStressed(act, measurement, snapshots, 0)

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
	act reasoning.Act,
	measurement types.Measurement,
	snapshots []types.Measurement,
	reservationCredit int,
) {
	if measurement.Last <= 0 {
		return
	}

	feePct := ledger.costs.feePct(act.Type)

	switch act.Type {
	case reasoning.ActionLimit, reasoning.ActionMarket, reasoning.ActionIceberg:
		entrySide := trading.Buy

		if act.Side == trading.Sell {
			entrySide = trading.Sell
		}

		if reasoning.IsMakerAction(act.Type) && ledger.costs.ExecutionStressEnabled {
			fraction, err := entryDeployFraction(ledger.costs, act, snapshots)

			if err != nil {
				errnie.Error(err, "replay: entry deploy fraction")
				ledger.fundBlocked++

				return
			}

			quoteCurrencyName := quoteCurrency(measurement.Symbol)
			capital := ledger.costs.WalletBalance(quoteCurrencyName)
			slot := execution.EntrySlotSpend(
				capital,
				fraction,
				feePct,
				ledger.walletCash(quoteCurrencyName),
			)
			quantity := slot / measurement.Last

			if quantity > 0 {
				slippagePct := flatSlippagePct(ledger.costs, measurement.SpreadBPS, snapshots)
				slippagePct += broker.ReplayMakerAdverseSlippagePct(
					measurement.SpreadBPS,
					executionStressMultiplier(snapshots),
				)

				ledger.queueMakerEntry(
					measurement.Symbol,
					entrySide,
					measurement.Last,
					quantity,
					feePct,
					slippagePct,
					fraction,
					measurement,
					snapshots,
				)

				return
			}
		}

		ledger.openEntry(
			measurement.Symbol,
			entrySide,
			act,
			measurement,
			snapshots,
			feePct,
			measurement.At,
			reservationCredit,
		)
	case reasoning.ActionSettlePosition:
		ledger.closePosition(measurement.Symbol, measurement, snapshots, feePct)
	case reasoning.ActionStopLoss,
		reasoning.ActionStopLossLimit,
		reasoning.ActionTakeProfit,
		reasoning.ActionTakeProfitLimit,
		reasoning.ActionTrailingStop,
		reasoning.ActionTrailingStopLimit:
		// Protective exit: rest the order; it fills only when the price path
		// breaches its trigger (checked each tick in checkTriggers).
		ledger.armTrigger(measurement.Symbol, act)
	case reasoning.ActionNone:
		return
	}
}
