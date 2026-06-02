package optimizer

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/market/perspectives"
)

const (
	// DefaultTakerFeePct is Kraken spot taker fee at the lowest volume tier (0.40%).
	DefaultTakerFeePct = 0.004
	// DefaultSlippagePct is half-spread crossing per side (5 bps).
	DefaultSlippagePct = 0.0005
	// DefaultReentryTickCooldown suppresses immediate re-entry churn on dense tapes.
	DefaultReentryTickCooldown = 500
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
	ledger := acquireReplayLedger(simulation.costs)

	defer releaseReplayLedger(ledger)

	ledger.reentryTickCooldown = scaleReentryTickCooldown(len(simulation.rows))

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
		ledger.onTick(row.Symbol)
	}

	return ReplayResult{
		Score:        ledger.totalReturn(simulation.lastPrices()),
		ClosedTrades: ledger.closedTrades,
	}
}

func scaleReentryTickCooldown(tapeLen int) int {
	if tapeLen <= 0 {
		return DefaultReentryTickCooldown
	}

	scaled := max(1, tapeLen/100)

	return min(scaled, DefaultReentryTickCooldown)
}

func (simulation *ReplaySimulation) resultFromTape() ReplayResult {
	ledger := acquireReplayLedger(simulation.costs)

	defer releaseReplayLedger(ledger)

	ledger.reentryTickCooldown = scaleReentryTickCooldown(simulation.tape.Len())

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
		ledger.onTick(tick.Row.Symbol)
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
