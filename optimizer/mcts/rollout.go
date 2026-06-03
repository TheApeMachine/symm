package mcts

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/replay"
)

/*
Rollout simulates playouts from expanded nodes and updates search rewards.
*/
type Rollout struct {
	search *TreeSearch
}

func NewRollout(search *TreeSearch) *Rollout {
	return &Rollout{search: search}
}

func (rollout *Rollout) Run(iteration int, node *Node) float64 {
	branches := node.branches.Clone()
	adversarial := rollout.shouldRunAdversarial(iteration)

	for step := 0; step < rollout.search.maxReasoningSteps; step++ {
		candidates := rollout.search.heuristic.Reachable(
			rollout.search.moves.Cached(),
			branches,
		)

		if len(candidates) == 0 {
			break
		}

		var move Move

		if adversarial && step == 0 {
			move = rollout.search.heuristic.SampleAdversarialMove(candidates, branches)
		} else {
			move = rollout.search.heuristic.SampleRolloutMove(candidates, branches)
		}

		branches = rollout.search.moves.Apply(branches, move)
	}

	score := rollout.search.evaluate(branches)
	closedTrades := 0

	if len(branches) > 0 {
		canonical := perspectives.CanonicalPlaybookBranches(branches)
		replay := replay.NewReplaySimulationWithTape(
			rollout.search.ctx, canonical, rollout.search.tape,
		).Result()
		closedTrades = replay.ClosedTrades

		if rollout.search.guard.ImprovesPersistedBest(
			score, replay.ClosedTrades, rollout.search.bestScore, rollout.search.bestClosedTrades,
		) && rollout.search.guard.PersistCandidate(canonical) {
			rollout.search.bestScore = score
			rollout.search.bestClosedTrades = replay.ClosedTrades
			rollout.search.best = canonical.Clone()
			rollout.search.emitBest(iteration, canonical, score)
		}
	}

	if rollout.search.progress != nil {
		rollout.search.progress.Record(
			score,
			closedTrades,
			rollout.search.guard.ImprovesPersistedBest,
		)
	}

	return normalizeReward(score, rollout.search.rewardScale)
}

func (rollout *Rollout) shouldRunAdversarial(iteration int) bool {
	fraction := rollout.search.budget.AdversarialRolloutFraction

	if fraction > 0 && rollout.search.rng.Float64() < fraction {
		return true
	}

	if rollout.search.budget.AdversarialRolloutInterval <= 0 {
		return false
	}

	return (iteration+1)%rollout.search.budget.AdversarialRolloutInterval == 0
}

func normalizeReward(pnlScore float64, rewardScale float64) float64 {
	scale := rewardScale

	if scale <= 0 {
		scale = 1
	}

	return 1.0 / (1.0 + math.Exp(-pnlScore*scale))
}
