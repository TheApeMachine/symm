package optimizer

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives"
)

func (search *TreeSearch) SetStagnationWindow(beamWidth int) {
	if beamWidth > 0 {
		search.stagnationWindow = beamWidth
	}
}

func (search *TreeSearch) scoreBranches(branches perspectives.BranchList) float64 {
	canonical := perspectives.CanonicalPlaybookBranches(branches)
	raw := NewReplaySimulationWithTape(
		search.ctx, canonical, search.tape,
	).Result().Score

	return search.guard.AdjustedScore(raw, canonical)
}

func (search *TreeSearch) rollout(iteration int, node *Node) float64 {
	branches := node.branches.Clone()
	adversarial := search.shouldRunAdversarialRollout(iteration)

	for step := 0; step < search.maxReasoningSteps; step++ {
		moves := search.reachableMoves(search.allMoves(), branches)

		if len(moves) == 0 {
			break
		}

		var move Move

		if adversarial && step == 0 {
			move = search.sampleAdversarialMove(moves, branches)
		} else {
			move = search.sampleRolloutMove(moves, branches)
		}

		branches = search.applyMove(branches, move)
	}

	score := search.evaluate(branches)
	closedTrades := 0

	if len(branches) > 0 {
		canonical := perspectives.CanonicalPlaybookBranches(branches)
		replay := NewReplaySimulationWithTape(
			search.ctx, canonical, search.tape,
		).Result()
		closedTrades = replay.ClosedTrades

		if search.guard.ImprovesPersistedBest(
			score, replay.ClosedTrades, search.bestScore, search.bestClosedTrades,
		) && search.guard.PersistCandidate(canonical) {
			search.bestScore = score
			search.bestClosedTrades = replay.ClosedTrades
			search.best = canonical.Clone()
			search.emitBest(iteration, canonical, score)
		}
	}

	if search.progress != nil {
		search.progress.Record(
			score,
			closedTrades,
			search.guard.ImprovesPersistedBest,
		)
	}

	return search.normalizeMCTSReward(score)
}

func (search *TreeSearch) emitBest(
	iteration int, branches perspectives.BranchList, score float64,
) {
	if search.onBest == nil {
		return
	}

	search.onBest(BestTree{
		Iteration: iteration + 1,
		Score:     score,
		Branches:  branches.Clone(),
	})
}

func (search *TreeSearch) shouldRunAdversarialRollout(iteration int) bool {
	fraction := search.budget.AdversarialRolloutFraction

	if fraction > 0 && search.rng.Float64() < fraction {
		return true
	}

	if search.budget.AdversarialRolloutInterval <= 0 {
		return false
	}

	return (iteration+1)%search.budget.AdversarialRolloutInterval == 0
}

func (search *TreeSearch) normalizeMCTSReward(pnlScore float64) float64 {
	scale := search.rewardScale

	if scale <= 0 {
		scale = 1
	}

	return 1.0 / (1.0 + math.Exp(-pnlScore*scale))
}

func (search *TreeSearch) moves(
	branches perspectives.BranchList,
) []Move {
	if reasoningDepth(branches) >= search.maxReasoningSteps {
		return nil
	}

	return search.reachableMoves(search.allMoves(), branches)
}
