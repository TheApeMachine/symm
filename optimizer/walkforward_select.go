package optimizer

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
SelectWalkForwardBest ranks finalists by average holdout return-per-trade, then
in-sample adjusted score, instead of discarding the entire run when the top IS
tree fails a binary walk-forward gate.
*/
func SelectWalkForwardBest(
	guard *OverfitGuard,
	rows []perspectives.Measurement,
	candidates []CandidateScore,
) perspectives.BranchList {
	candidates = dedupeCandidatesByBranch(candidates)

	if len(candidates) == 0 {
		return perspectives.BranchList{}
	}

	bestIndex := 0
	bestHoldout := math.Inf(-1)
	bestAdjusted := math.Inf(-1)

	for index, candidate := range candidates {
		if len(candidate.Branches) == 0 {
			continue
		}

		result := guard.EvaluateWalkForward(candidate.Branches, rows)
		holdout := result.AvgTestPerTrade()
		adjusted := candidate.AdjustedScore

		if holdout > bestHoldout ||
			(holdout == bestHoldout && adjusted > bestAdjusted) {
			bestIndex = index
			bestHoldout = holdout
			bestAdjusted = adjusted
		}
	}

	if len(candidates[bestIndex].Branches) == 0 {
		return perspectives.BranchList{}
	}

	return candidates[bestIndex].Branches.Clone()
}
