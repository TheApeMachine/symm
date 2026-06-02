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
	bestIndex := 0
	bestSupport := int(^uint(0) >> 1)

	for index, move := range moves {
		support := search.moveChainSupport(move, branches)

		if move.theoretical && support < bestSupport {
			bestSupport = support
			bestIndex = index
		}
	}

	if bestSupport < int(^uint(0)>>1) {
		return moves[bestIndex]
	}

	lowestSupport := int(^uint(0) >> 1)

	for index, move := range moves {
		support := search.moveChainSupport(move, branches)

		if support < lowestSupport {
			lowestSupport = support
			bestIndex = index
		}
	}

	return moves[bestIndex]
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
