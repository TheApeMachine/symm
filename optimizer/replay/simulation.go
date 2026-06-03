package replay

import (
	"context"
	"time"

	"github.com/theapemachine/symm/market/perspectives"
)

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
Score replays measurements and returns realized PnL from closed round trips.
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

	measurements := acquireReplayMeasurements()
	ledger := acquireReplayLedger(simulation.costs)

	defer releaseReplayMeasurements(measurements)
	defer releaseReplayLedger(ledger)

	ledger.reentryTickCooldown = reentryCooldownForRows(simulation.rows)
	medianInterval := medianMeasurementInterval(simulation.rows)
	ledger.configureExecutionStress(
		simulation.costs.effectiveExecutionLatency(simulation.rows, simulation.tape),
		medianInterval,
	)

	lastAt := time.Time{}
	lastRow := perspectives.Measurement{}

	for tickIndex, row := range simulation.rows {
		ledger.tickIndex = tickIndex
		ledger.onTickStart(row.At, row)
		measurements.Add(row)

		if row.Symbol == "" {
			continue
		}

		snapshotBuffer := acquireReplaySnapshotBuffer()
		snapshotBuffer = append(snapshotBuffer, measurements.Snapshot(row.Symbol)...)

		branchContext := simulation.branchContext(
			row,
			snapshotBuffer,
			ledger,
		)
		simulation.applyEvaluator(ledger, branchContext, row, snapshotBuffer)
		releaseReplaySnapshotBuffer(snapshotBuffer)
		ledger.onTick(row.Symbol)

		if !row.At.IsZero() {
			lastAt = row.At
			lastRow = row
		}
	}

	ledger.flushPending(lastAt, lastRow)

	return ReplayResult{
		Score:        ledger.realizedReturn(),
		ClosedTrades: ledger.closedTrades,
	}
}

func (simulation *ReplaySimulation) resultFromTape() ReplayResult {
	ledger := acquireReplayLedger(simulation.costs)

	defer releaseReplayLedger(ledger)

	ledger.reentryTickCooldown = simulation.tape.ReentryTickCooldown

	if ledger.reentryTickCooldown <= 0 {
		ledger.reentryTickCooldown = 1
	}

	medianInterval := simulation.tape.MedianInterval
	ledger.configureExecutionStress(
		simulation.costs.effectiveExecutionLatency(nil, simulation.tape),
		medianInterval,
	)

	lastAt := time.Time{}
	lastRow := perspectives.Measurement{}

	for tickIndex, tick := range simulation.tape.Ticks {
		ledger.tickIndex = tickIndex
		ledger.onTickStart(tick.Row.At, tick.Row)

		if tick.Row.Symbol == "" {
			continue
		}

		branchContext := simulation.branchContext(
			tick.Row,
			tick.Snapshots,
			ledger,
		)
		simulation.applyEvaluator(ledger, branchContext, tick.Row, tick.Snapshots)
		ledger.onTick(tick.Row.Symbol)

		if !tick.Row.At.IsZero() {
			lastAt = tick.Row.At
			lastRow = tick.Row
		}
	}

	ledger.flushPending(lastAt, lastRow)

	return ReplayResult{
		Score:        ledger.realizedReturn(),
		ClosedTrades: ledger.closedTrades,
	}
}

func reentryCooldownForRows(rows []perspectives.Measurement) int {
	categories := make(map[perspectives.CategoryType]struct{})

	for _, row := range rows {
		if row.Category == perspectives.CategoryTypeNone {
			continue
		}

		categories[row.Category] = struct{}{}
	}

	categoryCount := len(categories)

	if categoryCount <= 0 {
		categoryCount = 1
	}

	return deriveReentryTickCooldown(len(rows), categoryCount)
}

func (simulation *ReplaySimulation) applyEvaluator(
	ledger *replayLedger,
	branchContext perspectives.BranchContext,
	row perspectives.Measurement,
	snapshots []perspectives.Measurement,
) {
	evaluator := perspectives.NewBranchEvaluator(branchContext)
	actionType := evaluator.Action(simulation.branches)

	if evaluator.Err() != nil {
		return
	}

	if actionType == nil {
		return
	}

	ledger.queueAction(*actionType, row, snapshots)
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
