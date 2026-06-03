package guard

import (
	"context"
	"math"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/beam"
	"github.com/theapemachine/symm/optimizer/types"
)

/*
SelectWalkForwardBest ranks finalists by average holdout return-per-trade, then
in-sample adjusted score, instead of discarding the entire run when the top IS
tree fails a binary walk-forward gate.
*/
func SelectWalkForwardBest(
	guard *OverfitGuard,
	rows []perspectives.Measurement,
	candidates []types.CandidateScore,
	pool *qpool.Q,
) perspectives.BranchList {
	candidates = beam.DedupeCandidatesByBranch(candidates)

	if len(candidates) == 0 {
		return perspectives.BranchList{}
	}

	if pool != nil && len(candidates) > 1 {
		return selectWalkForwardBestWithPool(guard, rows, candidates, pool)
	}

	return selectWalkForwardBestSequential(guard, rows, candidates)
}

type walkForwardRank struct {
	index    int
	wins     int
	holdout  float64
	adjusted float64
}

func selectWalkForwardBestWithPool(
	guard *OverfitGuard,
	rows []perspectives.Measurement,
	candidates []types.CandidateScore,
	pool *qpool.Q,
) perspectives.BranchList {
	tasks := make([]chan *qpool.QValue[any], 0, len(candidates))

	for index, candidate := range candidates {
		if len(candidate.Branches) == 0 {
			continue
		}

		candidateIndex := index
		captured := candidate
		tasks = append(tasks, pool.ScheduleFast(guard.ctx, func(context.Context) (any, error) {
			result := guard.EvaluateWalkForward(captured.Branches, rows)
			wins := result.Wins

			if result.RegimeWins > wins {
				wins = result.RegimeWins
			}

			return walkForwardRank{
				index:    candidateIndex,
				wins:     wins,
				holdout:  result.AvgTestPerTrade(),
				adjusted: captured.AdjustedScore,
			}, nil
		}))
	}

	bestIndex := 0
	bestWins := -1
	bestHoldout := math.Inf(-1)
	bestAdjusted := math.Inf(-1)

	for _, task := range tasks {
		value := <-task

		if value.Error != nil {
			continue
		}

		rank, ok := value.Value.(walkForwardRank)

		if !ok {
			continue
		}

		if rank.wins > bestWins ||
			(rank.wins == bestWins && rank.holdout > bestHoldout) ||
			(rank.wins == bestWins && rank.holdout == bestHoldout && rank.adjusted > bestAdjusted) {
			bestIndex = rank.index
			bestWins = rank.wins
			bestHoldout = rank.holdout
			bestAdjusted = rank.adjusted
		}
	}

	if len(candidates[bestIndex].Branches) == 0 {
		return perspectives.BranchList{}
	}

	return candidates[bestIndex].Branches.Clone()
}

func selectWalkForwardBestSequential(
	guard *OverfitGuard,
	rows []perspectives.Measurement,
	candidates []types.CandidateScore,
) perspectives.BranchList {
	bestIndex := 0
	bestWins := -1
	bestHoldout := math.Inf(-1)
	bestAdjusted := math.Inf(-1)

	for index, candidate := range candidates {
		if len(candidate.Branches) == 0 {
			continue
		}

		result := guard.EvaluateWalkForward(candidate.Branches, rows)
		wins := result.Wins

		if result.RegimeWins > wins {
			wins = result.RegimeWins
		}

		holdout := result.AvgTestPerTrade()
		adjusted := candidate.AdjustedScore

		if wins > bestWins ||
			(wins == bestWins && holdout > bestHoldout) ||
			(wins == bestWins && holdout == bestHoldout && adjusted > bestAdjusted) {
			bestIndex = index
			bestWins = wins
			bestHoldout = holdout
			bestAdjusted = adjusted
		}
	}

	if len(candidates[bestIndex].Branches) == 0 {
		return perspectives.BranchList{}
	}

	return candidates[bestIndex].Branches.Clone()
}
