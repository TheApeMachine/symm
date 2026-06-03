package progress

import (
	"math"

	"github.com/theapemachine/symm/optimizer/playbook"
	"github.com/theapemachine/symm/optimizer/types"
)

/*
SearchProgress tracks whether scoring is still moving realized PnL forward.
The stagnation window equals the active beam width so the threshold scales with
search breadth instead of a fixed constant.
*/
type SearchProgress struct {
	bestScore        float64
	bestClosedTrades int
	sinceImprovement int
}

func NewSearchProgress() *SearchProgress {
	return &SearchProgress{
		bestScore:        math.Inf(-1),
		bestClosedTrades: -1,
	}
}

func (progress *SearchProgress) BestScore() float64 {
	return progress.bestScore
}

func (progress *SearchProgress) SinceImprovement() int {
	return progress.sinceImprovement
}

func (progress *SearchProgress) StagnationLimit(beamWidth int) int {
	if beamWidth <= 0 {
		return 1
	}

	return beamWidth
}

func (progress *SearchProgress) Record(
	adjustedScore float64,
	closedTrades int,
	improves func(float64, int, float64, int) bool,
) bool {
	if !improves(
		adjustedScore, closedTrades,
		progress.bestScore, progress.bestClosedTrades,
	) {
		progress.sinceImprovement++

		return false
	}

	progress.bestScore = adjustedScore
	progress.bestClosedTrades = closedTrades
	progress.sinceImprovement = 0

	return true
}

func (progress *SearchProgress) Stagnant(beamWidth int) bool {
	return progress.sinceImprovement >= progress.StagnationLimit(beamWidth)
}

func (progress *SearchProgress) ResetStagnation() {
	progress.sinceImprovement = 0
}

func maxReasoningDepthInBeam(beam []types.CandidateScore) int {
	maxDepth := 0

	for _, candidate := range beam {
		depth := playbook.ReasoningDepth(candidate.Branches)

		if depth > maxDepth {
			maxDepth = depth
		}
	}

	return maxDepth
}

/*
seedSearchTargetDepth is the shallowest reasoning depth among beam survivors with
closed round trips. Deny-wrapper nesting can inflate maxReasoningDepthInBeam while
traded seeds stay shallow; search depth must advance from the traded baseline.
*/
func SeedSearchTargetDepth(beam []types.CandidateScore) int {
	targetDepth := 0

	for _, candidate := range beam {
		if candidate.ClosedTrades <= 0 {
			continue
		}

		depth := playbook.ReasoningDepth(candidate.Branches)

		if targetDepth == 0 || depth < targetDepth {
			targetDepth = depth
		}
	}

	if targetDepth <= 0 {
		return 1
	}

	return targetDepth
}
