package optimizer

import (
	"context"
	"math"
	"sort"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/market/perspectives"
)

const (
	// DefaultTakerFeePct is Kraken spot taker fee at the lowest volume tier (0.40%).
	DefaultTakerFeePct = 0.004
	// DefaultSlippagePct is half-spread crossing per side (5 bps).
	DefaultSlippagePct = 0.0005
)

/*
ReplayCosts models per-side execution drag for offline replay scoring.
*/
type ReplayCosts struct {
	TakerFeePct float64
	SlippagePct float64
}

/*
DefaultReplayCosts returns conservative Kraken spot assumptions for tuning.
*/
func DefaultReplayCosts() ReplayCosts {
	return ReplayCosts{
		TakerFeePct: DefaultTakerFeePct,
		SlippagePct: DefaultSlippagePct,
	}
}

/*
ReplaySimulation walks a candidate tree across collected replay measurements.
*/
type ReplaySimulation struct {
	ctx      context.Context
	branches perspectives.BranchList
	rows     []perspectives.Measurement
	tape     ReplayTape
	costs    ReplayCosts
}

func NewReplaySimulation(
	ctx context.Context,
	branches perspectives.BranchList,
	rows []perspectives.Measurement,
) *ReplaySimulation {
	return NewReplaySimulationWithTapeAndCosts(
		ctx, branches, PrecompileTape(rows), DefaultReplayCosts(),
	)
}

/*
NewReplaySimulationWithTape replays against a shared precompiled measurement tape.
*/
func NewReplaySimulationWithTape(
	ctx context.Context,
	branches perspectives.BranchList,
	tape ReplayTape,
) *ReplaySimulation {
	return NewReplaySimulationWithTapeAndCosts(
		ctx, branches, tape, DefaultReplayCosts(),
	)
}

/*
NewReplaySimulationWithCosts replays with explicit fee and slippage assumptions.
*/
func NewReplaySimulationWithCosts(
	ctx context.Context,
	branches perspectives.BranchList,
	rows []perspectives.Measurement,
	costs ReplayCosts,
) *ReplaySimulation {
	return NewReplaySimulationWithTapeAndCosts(
		ctx, branches, PrecompileTape(rows), costs,
	)
}

/*
NewReplaySimulationWithTapeAndCosts replays a shared tape with explicit costs.
*/
func NewReplaySimulationWithTapeAndCosts(
	ctx context.Context,
	branches perspectives.BranchList,
	tape ReplayTape,
	costs ReplayCosts,
) *ReplaySimulation {
	return &ReplaySimulation{
		ctx:      ctx,
		branches: perspectives.CanonicalPlaybookBranches(branches),
		tape:     tape,
		costs:    costs,
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
ReturnPerTrade is mean net PnL per closed round trip.
*/
func (result ReplayResult) ReturnPerTrade() float64 {
	if result.ClosedTrades <= 0 {
		return 0
	}

	return result.Score / float64(result.ClosedTrades)
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
	if simulation.tape.Len() > 0 {
		return simulation.resultFromTape()
	}

	if len(simulation.rows) == 0 {
		return ReplayResult{}
	}

	measurements := newReplayMeasurements()
	ledger := newReplayLedger(simulation.costs)

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
		simulation.applyEvaluator(ledger, branchContext, row)
	}

	return ReplayResult{
		Score:        ledger.totalReturn(simulation.lastPrices()),
		ClosedTrades: ledger.closedTrades,
	}
}

func (simulation *ReplaySimulation) resultFromTape() ReplayResult {
	ledger := newReplayLedger(simulation.costs)

	for _, tick := range simulation.tape.Ticks {
		if tick.Row.Symbol == "" {
			continue
		}

		branchContext := simulation.branchContext(
			tick.Row,
			tick.Snapshots,
			ledger,
		)
		simulation.applyEvaluator(ledger, branchContext, tick.Row)
	}

	return ReplayResult{
		Score:        ledger.totalReturn(simulation.tape.LastPrices),
		ClosedTrades: ledger.closedTrades,
	}
}

func (simulation *ReplaySimulation) applyEvaluator(
	ledger *replayLedger,
	branchContext perspectives.BranchContext,
	row perspectives.Measurement,
) {
	evaluator := perspectives.NewBranchEvaluator(branchContext)
	actionType := evaluator.Action(simulation.branches)

	if evaluator.Err() != nil {
		errnie.Error(evaluator.Err())
	}

	if actionType == nil {
		return
	}

	ledger.apply(*actionType, row)
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
	costs        ReplayCosts
	positions    map[string]replayPosition
	realized     float64
	closedTrades int
}

func newReplayLedger(costs ReplayCosts) *replayLedger {
	return &replayLedger{
		costs:     costs,
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

	entryFill := price * (1 + ledger.costs.SlippagePct)

	ledger.positions[symbol] = replayPosition{
		entryPrice: entryFill,
		quantity:   1,
	}

	ledger.realized -= ledger.costs.TakerFeePct
}

func (ledger *replayLedger) closeLong(symbol string, price float64) {
	position, open := ledger.positions[symbol]

	if !open || position.entryPrice <= 0 {
		return
	}

	exitFill := price * (1 - ledger.costs.SlippagePct)
	gross := (exitFill - position.entryPrice) / position.entryPrice

	ledger.realized += gross - ledger.costs.TakerFeePct
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

	exitFill := measurement.Last * (1 - ledger.costs.SlippagePct)
	change := exitFill - position.entryPrice
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

		exitFill := lastPrice * (1 - ledger.costs.SlippagePct)
		total += (exitFill - position.entryPrice) / position.entryPrice
	}

	return total
}

func holdoutDecay(trainPerTrade float64, testPerTrade float64) float64 {
	if trainPerTrade <= 0 {
		return math.Inf(1)
	}

	return (trainPerTrade - testPerTrade) / trainPerTrade
}
