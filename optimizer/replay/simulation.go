package replay

import (
	"context"
	"time"

	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
ReplaySimulation scores a reasoning forest (Thought trees) against a precompiled
measurement tape, driving the shared cash + trigger ledger from EvaluateStateful so
per-node offsets, lifecycle subjects, and the latched temporal walk all take effect.
*/
type ReplaySimulation struct {
	ctx      context.Context
	thoughts []reasoning.Thought
	tape     ReplayTape
	costs    ReplayCosts
}

/*
NewThoughtSimulation scores a reasoning-language playbook against a tape.
*/
func NewThoughtSimulation(
	ctx context.Context,
	thoughts []reasoning.Thought,
	tape ReplayTape,
	costs ReplayCosts,
) *ReplaySimulation {
	// Generated forests arrive unnamed; stamping here gives every candidate the
	// same per-setup attribution hand-written files get from ParseThoughts.
	reasoning.StampStrategies(thoughts)

	return &ReplaySimulation{
		ctx:      ctx,
		thoughts: thoughts,
		tape:     tape,
		costs:    costs,
	}
}

/*
ReplayResult holds realized PnL and round-trip activity from one replay pass.
*/
type ReplayResult struct {
	Score           float64
	RealizedEUR     float64
	ClosedTrades    int
	ExposureTicks   int
	TotalTicks      int
	StartingCapital float64
	// FundBlocked is how many times an entry was wanted on a fundable pair but the
	// wallet was already locked in another position — the opportunity cost of
	// tying up capital, surfaced so the optimizer can price it.
	FundBlocked int
	// PreflightBlocked / ExitBlocked surface where wanted trades died, so a
	// zero-trade tune is diagnosable from its log instead of a mystery.
	PreflightBlocked int
	ExitBlocked      int
	// PerStrategy attributes round trips to the named setup that entered them —
	// the forest stops being one anonymous blob and becomes a portfolio of
	// setups, each accountable on its own line.
	PerStrategy map[string]StrategyResult
	// Trades is the attributed round-trip list, populated only when
	// ReplayCosts.CollectTrades is set (the workbench path).
	Trades []ClosedTrade
}

/*
StrategyResult is one setup's attributed results over a replay pass.
*/
type StrategyResult struct {
	Trades         int
	Wins           int
	RealizedEUR    float64
	AvgHoldSeconds float64
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
Score replays the tape and returns realized PnL from closed round trips.
*/
func (simulation *ReplaySimulation) Score() float64 {
	return simulation.SearchResult().Score
}

/*
SearchResult replays the tape and returns the fields the optimizer scores on,
without building per-setup attribution or trade logs.
*/
func (simulation *ReplaySimulation) SearchResult() ReplayResult {
	ledger := simulation.replay()

	if ledger == nil {
		return ReplayResult{}
	}

	defer releaseReplayLedger(ledger)

	return simulation.coreResult(ledger)
}

/*
Result replays the tape and returns PnL with trade activity.
*/
func (simulation *ReplaySimulation) Result() ReplayResult {
	ledger := simulation.replay()

	if ledger == nil {
		return ReplayResult{}
	}

	defer releaseReplayLedger(ledger)

	result := simulation.coreResult(ledger)

	if !simulation.costs.CollectAttribution && !simulation.costs.CollectTrades {
		return result
	}

	if simulation.costs.CollectAttribution {
		result.PerStrategy = attributedStrategies(ledger)
	}

	if simulation.costs.CollectTrades {
		result.Trades = append([]ClosedTrade(nil), ledger.tradeLog...)
	}

	return result
}

func (simulation *ReplaySimulation) replay() *replayLedger {
	if simulation.tape.Len() == 0 {
		return nil
	}

	ledger := acquireReplayLedger(simulation.costs)

	ledger.reentryTickCooldown = simulation.tape.ReentryTickCooldown

	if ledger.reentryTickCooldown <= 0 {
		ledger.reentryTickCooldown = 1
	}

	ledger.configureExecutionStress(
		simulation.costs.effectiveExecutionLatency(),
		simulation.tape.MedianInterval,
	)

	lastAt := time.Time{}
	lastRow := types.Measurement{}
	reason := &ledger.windowReason
	snapshots := ledger.snapshotScratch[:0]

	defer func() {
		ledger.snapshotScratch = snapshots[:0]
	}()

	for tickIndex, tick := range simulation.tape.Ticks {
		ledger.tickIndex = tickIndex
		ledger.exposureTicks += len(ledger.positions)
		ledger.onTickStart(tick.Row.At, tick.Row)

		if tick.Row.Symbol == "" {
			continue
		}

		snapshots = simulation.tape.AppendSnapshot(tickIndex, snapshots)
		simulation.applyThoughts(ledger, tick, snapshots, reason)
		ledger.onTick(tick.Row.Symbol)

		if !tick.Row.At.IsZero() {
			lastAt = tick.Row.At
			lastRow = tick.Row
		}
	}

	ledger.flushEntryBatch(time.Time{})
	ledger.flushPending(lastAt, lastRow)

	return ledger
}

func (simulation *ReplaySimulation) coreResult(ledger *replayLedger) ReplayResult {
	return ReplayResult{
		Score:            ledger.realizedReturn(),
		RealizedEUR:      ledger.realized,
		ClosedTrades:     ledger.closedTrades,
		ExposureTicks:    ledger.exposureTicks,
		TotalTicks:       simulation.tape.Len(),
		StartingCapital:  ledger.startingCapital(),
		FundBlocked:      ledger.fundBlocked,
		PreflightBlocked: ledger.preflightBlocked,
		ExitBlocked:      ledger.exitBlocked,
	}
}

func attributedStrategies(ledger *replayLedger) map[string]StrategyResult {
	perStrategy := make(map[string]StrategyResult, len(ledger.perStrategy))

	for strategy, tally := range ledger.perStrategy {
		attributed := StrategyResult{
			Trades:      tally.trades,
			Wins:        tally.wins,
			RealizedEUR: tally.realizedEUR,
		}

		if tally.trades > 0 {
			attributed.AvgHoldSeconds = tally.holdSeconds / float64(tally.trades)
		}

		perStrategy[strategy] = attributed
	}

	return perStrategy
}

/*
applyThoughts drives the ledger from the reasoning language: build the per-tick
context (window + regime + open position), evaluate the thoughts with the symbol's
cross-tick state, and queue the chosen act (carrying its per-node trigger offset).
*/
func (simulation *ReplaySimulation) applyThoughts(
	ledger *replayLedger,
	tick PrecompiledTick,
	snapshots []types.Measurement,
	reason *reasoning.WindowReason,
) {
	regime := tick.Regime
	reason.Reset(snapshots, regime, ledger.positionState(tick.Row))
	ledger.lastRegime = regime

	act, firedKey, found := reasoning.EvaluateStatefulKeyed(
		simulation.thoughts, reason, ledger.reasonState(tick.Row.Symbol),
	)

	if !found || act.Type == reasoning.ActionNone {
		return
	}

	row := tick.Row

	// Same conviction attribution as live: the firing leaf's evidence, not the
	// ambient row that happened to trigger the walk.
	row.SNR, row.Confidence = reasoning.ResolveConviction(
		simulation.thoughts, firedKey, reason, row,
	)

	ledger.queueAction(act, row, snapshots)
}
