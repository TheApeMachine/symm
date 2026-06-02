package optimizer

import (
	"github.com/theapemachine/symm/market/perspectives"
)

/*
sampleAdversarialMove prefers theoretical or low-support category chains so
MCTS maps defensive guardrails against unobserved signal combinations.
*/
func (search *TreeSearch) sampleAdversarialMove(
	moves []Move, branches perspectives.BranchList,
) Move {
	chosen := moves[0]
	lowestReachScore := 2.0

	for _, move := range moves {
		reachScore := search.moveChainReachabilityScore(move, branches)

		if !move.theoretical {
			continue
		}

		if reachScore < lowestReachScore {
			lowestReachScore = reachScore
			chosen = move
		}
	}

	if lowestReachScore < 1 {
		return chosen
	}

	lowestSupport := int(^uint(0) >> 1)

	for _, move := range moves {
		support := search.moveChainSupport(move, branches)

		if support < lowestSupport {
			lowestSupport = support
			chosen = move
		}
	}

	return chosen
}

func (search *TreeSearch) moveChainReachabilityScore(
	move Move, branches perspectives.BranchList,
) float64 {
	if search.coOccurrence == nil {
		return 1
	}

	chain := search.moveChainCategories(move, branches)

	return search.coOccurrence.ChainReachabilityScore(chain)
}

func (search *TreeSearch) moveChainSupport(
	move Move, branches perspectives.BranchList,
) int {
	if search.coOccurrence == nil {
		return 0
	}

	chain := search.moveChainCategories(move, branches)

	return search.coOccurrence.chainSupport(chain)
}
