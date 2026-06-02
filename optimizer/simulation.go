package optimizer

import (
	"context"
	"sort"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
ReplaySimulation walks a candidate tree across collected replay measurements.
*/
type ReplaySimulation struct {
	ctx      context.Context
	branches perspectives.BranchList
	rows     []perspectives.Measurement
}

func NewReplaySimulation(
	ctx context.Context,
	branches perspectives.BranchList,
	rows []perspectives.Measurement,
) *ReplaySimulation {
	return &ReplaySimulation{
		ctx:      ctx,
		branches: branches.Clone(),
		rows:     rows,
	}
}

/*
ReplayResult holds realized PnL and round-trip activity from one replay pass.
*/
type ReplayResult struct {
	Score        float64
	ClosedTrades int
}

/*
Score replays measurements and returns realized plus marked PnL.
*/
func (simulation *ReplaySimulation) Score() float64 {
	return simulation.Result().Score
}

/*
Result replays measurements and returns PnL with trade activity.
*/
func (simulation *ReplaySimulation) Result() ReplayResult {
	if len(simulation.rows) == 0 {
		return ReplayResult{}
	}

	measurements := newReplayMeasurements()
	ledger := newReplayLedger()

	for _, row := range simulation.rows {
		measurements.Add(row)

		if row.Symbol == "" {
			continue
		}

		branchContext := simulation.branchContext(
			row,
			measurements.Snapshot(row.Symbol),
			ledger,
		)
		evaluator := perspectives.NewBranchEvaluator(branchContext)
		actionType := evaluator.Action(simulation.branches)

		if evaluator.Err() != nil {
			errnie.Error(evaluator.Err())
		}

		if actionType == nil {
			continue
		}

		ledger.apply(*actionType, row)
	}

	return ReplayResult{
		Score:        ledger.totalReturn(simulation.lastPrices()),
		ClosedTrades: ledger.closedTrades,
	}
}

func (simulation *ReplaySimulation) branchContext(
	row perspectives.Measurement,
	measurements []perspectives.Measurement,
	ledger *replayLedger,
) perspectives.BranchContext {
	return perspectives.BranchContext{
		Measurements: measurements,
		Observations: ledger.observations(row.Symbol),
		Metrics:      ledger.metrics(row),
	}
}

func (simulation *ReplaySimulation) lastPrices() map[string]float64 {
	lastPrices := make(map[string]float64)

	for _, row := range simulation.rows {
		if row.Symbol == "" || row.Last <= 0 {
			continue
		}

		lastPrices[row.Symbol] = row.Last
	}

	return lastPrices
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

	rows = appendMeasurementsBySource(rows, measurements.global)
	rows = appendMeasurementsBySource(rows, measurements.symbols[symbol])

	return rows
}

func appendMeasurementsBySource(
	rows []perspectives.Measurement,
	bySource map[perspectives.SourceType]perspectives.Measurement,
) []perspectives.Measurement {
	sources := make([]perspectives.SourceType, 0, len(bySource))

	for source := range bySource {
		sources = append(sources, source)
	}

	sort.Slice(sources, func(leftIndex, rightIndex int) bool {
		return sources[leftIndex] < sources[rightIndex]
	})

	for _, source := range sources {
		rows = append(rows, bySource[source])
	}

	return rows
}

type replayPosition struct {
	entryPrice float64
	quantity   float64
}

type replayLedger struct {
	positions    map[string]replayPosition
	realized     float64
	closedTrades int
}

func newReplayLedger() *replayLedger {
	return &replayLedger{
		positions: make(map[string]replayPosition),
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
		ledger.openLong(measurement.Symbol, measurement.Last)
	case perspectives.ActionSettlePosition,
		perspectives.ActionStopLoss,
		perspectives.ActionStopLossLimit,
		perspectives.ActionTakeProfit,
		perspectives.ActionTakeProfitLimit,
		perspectives.ActionTrailingStop,
		perspectives.ActionTrailingStopLimit:
		ledger.closeLong(measurement.Symbol, measurement.Last)
	case perspectives.ActionNone:
		return
	}
}

func (ledger *replayLedger) openLong(symbol string, price float64) {
	if _, open := ledger.positions[symbol]; open {
		return
	}

	ledger.positions[symbol] = replayPosition{
		entryPrice: price,
		quantity:   1,
	}
}

func (ledger *replayLedger) closeLong(symbol string, price float64) {
	position, open := ledger.positions[symbol]

	if !open || position.entryPrice <= 0 {
		return
	}

	ledger.realized += (price - position.entryPrice) / position.entryPrice
	ledger.closedTrades++
	delete(ledger.positions, symbol)
}

func (ledger *replayLedger) observations(
	symbol string,
) map[perspectives.ObservationType]float64 {
	observations := make(map[perspectives.ObservationType]float64, 1)

	if ledger.holding(symbol) {
		observations[perspectives.ObservationHolding] = 1

		return observations
	}

	observations[perspectives.ObservationNotHolding] = 1

	return observations
}

func (ledger *replayLedger) metrics(
	measurement perspectives.Measurement,
) map[string]float64 {
	metrics := map[string]float64{
		"last": measurement.Last,
	}

	position, open := ledger.positions[measurement.Symbol]

	if !open || position.entryPrice <= 0 || measurement.Last <= 0 {
		return metrics
	}

	change := measurement.Last - position.entryPrice
	metrics["unrealized_return"] = (change / position.entryPrice) * 100

	return metrics
}

func (ledger *replayLedger) holding(symbol string) bool {
	_, open := ledger.positions[symbol]

	return open
}

func (ledger *replayLedger) totalReturn(lastPrices map[string]float64) float64 {
	total := ledger.realized

	for symbol, position := range ledger.positions {
		lastPrice, ok := lastPrices[symbol]

		if !ok || lastPrice <= 0 || position.entryPrice <= 0 {
			continue
		}

		total += (lastPrice - position.entryPrice) / position.entryPrice
	}

	return total
}
