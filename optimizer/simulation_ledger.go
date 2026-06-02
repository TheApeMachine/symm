package optimizer

import (
	"math"
	"sort"
	"sync"

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
	entryPrice float64
	quantity   float64
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
}

func newReplayLedger(costs ReplayCosts) *replayLedger {
	return &replayLedger{
		costs:               costs,
		positions:           make(map[string]replayPosition),
		reentryTickCooldown: DefaultReentryTickCooldown,
		ticksSinceClose:     make(map[string]int),
		observationScratch:  make(map[perspectives.ObservationType]float64, 1),
		metricsScratch:      make(map[string]float64, 2),
	}
}

func (ledger *replayLedger) reset(costs ReplayCosts) {
	ledger.costs = costs
	ledger.reentryTickCooldown = DefaultReentryTickCooldown
	ledger.realized = 0
	ledger.closedTrades = 0

	clear(ledger.positions)
	clear(ledger.ticksSinceClose)
	clear(ledger.observationScratch)
	clear(ledger.metricsScratch)
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
	if measurement.Last <= 0 {
		return
	}

	switch actionType {
	case perspectives.ActionLimit, perspectives.ActionMarket, perspectives.ActionIceberg:
		ledger.openLong(
			measurement.Symbol,
			measurement.Last,
			measurement.SpreadBPS,
			ledger.costs.feePct(actionType),
		)
	case perspectives.ActionSettlePosition,
		perspectives.ActionStopLoss,
		perspectives.ActionStopLossLimit,
		perspectives.ActionTakeProfit,
		perspectives.ActionTakeProfitLimit,
		perspectives.ActionTrailingStop,
		perspectives.ActionTrailingStopLimit:
		ledger.closeLong(
			measurement.Symbol,
			measurement.Last,
			measurement.SpreadBPS,
			ledger.costs.feePct(actionType),
		)
	case perspectives.ActionNone:
		return
	}
}

func (ledger *replayLedger) openLong(
	symbol string,
	price float64,
	spreadBPS float64,
	feePct float64,
) {
	if _, open := ledger.positions[symbol]; open {
		return
	}

	if ticksSinceClose, tracked := ledger.ticksSinceClose[symbol]; tracked &&
		ticksSinceClose < ledger.reentryTickCooldown {
		return
	}

	slippagePct := halfSpreadSlippagePct(ledger.costs, spreadBPS)
	entryFill := price * (1 + slippagePct)

	ledger.positions[symbol] = replayPosition{
		entryPrice: entryFill,
		quantity:   1,
	}

	ledger.realized -= feePct
	delete(ledger.ticksSinceClose, symbol)
}

func (ledger *replayLedger) closeLong(
	symbol string,
	price float64,
	spreadBPS float64,
	feePct float64,
) {
	position, open := ledger.positions[symbol]

	if !open || position.entryPrice <= 0 {
		return
	}

	slippagePct := halfSpreadSlippagePct(ledger.costs, spreadBPS)
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

func holdoutDecay(trainPerTrade float64, testPerTrade float64) float64 {
	if trainPerTrade <= 0 {
		return math.Inf(1)
	}

	return (trainPerTrade - testPerTrade) / trainPerTrade
}
