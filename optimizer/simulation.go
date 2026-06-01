package optimizer

import (
	"context"
	"sort"

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
Score replays measurements and returns realized plus marked PnL.
*/
func (simulation *ReplaySimulation) Score() float64 {
	if len(simulation.rows) == 0 {
		return 0
	}

	measurements := newReplayMeasurements()
	ledger := newReplayLedger()

	for _, row := range simulation.rows {
		measurements.Add(row)

		if row.Symbol == "" {
			continue
		}

		actionType := simulation.action(measurements.Snapshot(row.Symbol))

		if actionType == nil {
			continue
		}

		ledger.apply(*actionType, row)
	}

	return ledger.totalReturn(simulation.lastPrices())
}

type replayDecision struct {
	actionType perspectives.ActionType
	depth      int
	found      bool
}

func (simulation *ReplaySimulation) action(
	measurements []perspectives.Measurement,
) *perspectives.ActionType {
	decision := simulation.walk(
		measurements,
		simulation.branches,
		0,
	)

	if !decision.found {
		return nil
	}

	return &decision.actionType
}

func (simulation *ReplaySimulation) walk(
	measurements []perspectives.Measurement,
	branches perspectives.BranchList,
	depth int,
) replayDecision {
	best := replayDecision{}

	for _, branch := range branches {
		measurement, ok := simulation.measurement(measurements, branch)

		if !ok {
			continue
		}

		if !simulation.passes(measurement, branch) {
			continue
		}

		if branch.Action.Type != perspectives.ActionNone && depth >= best.depth {
			best = replayDecision{
				actionType: branch.Action.Type,
				depth:      depth,
				found:      true,
			}
		}

		child := simulation.walk(
			measurements,
			perspectives.BranchList(branch.Branches),
			depth+1,
		)

		if child.found && child.depth > best.depth {
			best = child
		}
	}

	return best
}

func (simulation *ReplaySimulation) measurement(
	measurements []perspectives.Measurement,
	branch perspectives.Branch,
) (perspectives.Measurement, bool) {
	if branch.Category == perspectives.CategoryTypeNone {
		return perspectives.Measurement{}, true
	}

	for _, measurement := range measurements {
		if measurement.Category == branch.Category {
			return measurement, true
		}
	}

	return perspectives.Measurement{}, false
}

func (simulation *ReplaySimulation) passes(
	measurement perspectives.Measurement,
	branch perspectives.Branch,
) bool {
	if branch.Condition == perspectives.ConditionNone || branch.Unit == perspectives.UnitNone {
		return true
	}

	switch branch.Unit {
	case perspectives.UnitSNR:
		return simulation.compare(measurement.SNR, branch.Value, branch.Condition)
	case perspectives.UnitConfidence:
		return simulation.compare(measurement.Confidence, branch.Value, branch.Condition)
	default:
		return false
	}
}

func (simulation *ReplaySimulation) compare(
	left, right float64,
	condition perspectives.ConditionType,
) bool {
	switch condition {
	case perspectives.ConditionIsGreaterThan:
		return left > right
	case perspectives.ConditionIsLessThan:
		return left < right
	case perspectives.ConditionIsEqual:
		return left == right
	case perspectives.ConditionIsNotEqual:
		return left != right
	case perspectives.ConditionIsGreaterThanOrEqual:
		return left >= right
	case perspectives.ConditionIsLessThanOrEqual:
		return left <= right
	default:
		return false
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
	positions map[string]replayPosition
	realized  float64
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
	delete(ledger.positions, symbol)
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
