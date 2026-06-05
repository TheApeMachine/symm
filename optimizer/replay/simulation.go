package replay

import (
	"context"
	"time"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
ReplaySimulation scores a reasoning forest (Thought trees) against a precompiled
measurement tape, driving the shared cash + trigger ledger from EvaluateStateful so
per-node offsets, lifecycle subjects, and the latched temporal walk all take effect.
*/
type ReplaySimulation struct {
	ctx      context.Context
	thoughts []perspectives.Thought
	tape     ReplayTape
	costs    ReplayCosts
}

/*
NewThoughtSimulation scores a reasoning-language playbook against a tape.
*/
func NewThoughtSimulation(
	ctx context.Context,
	thoughts []perspectives.Thought,
	tape ReplayTape,
	costs ReplayCosts,
) *ReplaySimulation {
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
	Score        float64
	ClosedTrades int
	// FundBlocked is how many times an entry was wanted on a fundable pair but the
	// wallet was already locked in another position — the opportunity cost of
	// tying up capital, surfaced so the optimizer can price it.
	FundBlocked int
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
	return simulation.Result().Score
}

/*
Result replays the tape and returns PnL with trade activity.
*/
func (simulation *ReplaySimulation) Result() ReplayResult {
	if simulation.tape.Len() == 0 {
		return ReplayResult{}
	}

	ledger := acquireReplayLedger(simulation.costs)

	defer releaseReplayLedger(ledger)

	ledger.reentryTickCooldown = simulation.tape.ReentryTickCooldown

	if ledger.reentryTickCooldown <= 0 {
		ledger.reentryTickCooldown = 1
	}

	ledger.configureExecutionStress(
		simulation.costs.effectiveExecutionLatency(),
		simulation.tape.MedianInterval,
	)

	lastAt := time.Time{}
	lastRow := perspectives.Measurement{}
	reason := &ledger.windowReason
	snapshots := ledger.snapshotScratch[:0]

	defer func() {
		ledger.snapshotScratch = snapshots[:0]
	}()

	for tickIndex, tick := range simulation.tape.Ticks {
		ledger.tickIndex = tickIndex
		ledger.onTickStart(tick.Row.At, tick.Row)

		if tick.Row.Symbol == "" {
			continue
		}

		snapshots = simulation.tape.AppendSnapshot(tickIndex, snapshots)
		simulation.applyThoughts(ledger, tick.Row, snapshots, reason)
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
		FundBlocked:  ledger.fundBlocked,
	}
}

/*
applyThoughts drives the ledger from the reasoning language: build the per-tick
context (window + regime + open position), evaluate the thoughts with the symbol's
cross-tick state, and queue the chosen act (carrying its per-node trigger offset).
*/
func (simulation *ReplaySimulation) applyThoughts(
	ledger *replayLedger,
	row perspectives.Measurement,
	snapshots []perspectives.Measurement,
	reason *perspectives.WindowReason,
) {
	regime := perspectives.ClassifyRegime(snapshots).Regime
	reason.Reset(snapshots, regime, ledger.positionState(row))

	act, found := perspectives.EvaluateStateful(
		simulation.thoughts, reason, ledger.reasonState(row.Symbol),
	)

	if !found || act.Type == perspectives.ActionNone {
		return
	}

	ledger.queueAction(act, row, snapshots)
}
