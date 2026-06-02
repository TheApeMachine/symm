package optimizer

import (
	"github.com/theapemachine/symm/market/perspectives"
)

func (search *TreeSearch) branchFromMove(move Move) perspectives.Branch {
	return perspectives.Branch{
		Category:    move.category,
		Observation: move.observation,
		Regime:      move.regime,
		Condition:   move.condition,
		Unit:        move.unit,
		Value:       move.value,
		ValueSet:    true,
		Action: perspectives.Action{
			Type: move.action,
		},
	}
}

func (search *TreeSearch) moveReachable(
	move Move, branches perspectives.BranchList,
) bool {
	if search.profile.categoryCount(move.category) == 0 {
		return false
	}

	if search.coOccurrence != nil {
		if !search.moveChainReachable(move, branches) {
			return false
		}
	}

	return search.profile.GatePassCount(
		move.category, move.unit, move.condition, move.value,
	) > 0
}

func (search *TreeSearch) moveChainReachable(
	move Move, branches perspectives.BranchList,
) bool {
	switch move.observation {
	case perspectives.ObservationNone:
		return search.coOccurrence.ChainReachable(
			append(entryPathCategories(branches), move.category),
		)
	case perspectives.ObservationNotHolding:
		if len(branches) == 0 {
			return search.coOccurrence.ChainReachable(
				[]perspectives.CategoryType{move.category},
			)
		}

		return search.coOccurrence.ChainReachable(
			append(entryPathCategories(branches), move.category),
		)
	case perspectives.ObservationHolding:
		return search.coOccurrence.CategoriesReachable(
			[]perspectives.CategoryType{move.category},
		)
	default:
		return search.coOccurrence.ChainReachable(
			append(categoriesInBranchList(branches), move.category),
		)
	}
}

func (search *TreeSearch) moveCompatible(
	branches perspectives.BranchList, move Move,
) bool {
	if len(branches) == 0 {
		return true
	}

	branch := search.branchFromMove(move)

	switch move.observation {
	case perspectives.ObservationNone:
		if entryIndex := perspectives.FindEntryIndex(branches); entryIndex >= 0 {
			entry := branches[entryIndex]

			if anchor, ok := lastEntryChainGate(entry); ok {
				return isBranchCompatible(anchor, branch)
			}
		}

		if anchor, ok := lastGateBranch(branches); ok {
			return isBranchCompatible(anchor, branch)
		}

		return true
	default:
		return true
	}
}

func (search *TreeSearch) moveWeightForBranches(
	move Move, branches perspectives.BranchList,
) float64 {
	passes := search.profile.GatePassCount(
		move.category, move.unit, move.condition, move.value,
	)

	if passes == 0 {
		return 0
	}

	categoryTotal := search.profile.categoryCount(move.category)

	if categoryTotal == 0 || search.profile.Len() == 0 {
		return 0
	}

	categoryFrac := float64(categoryTotal) / float64(search.profile.Len())
	passRate := float64(passes) / float64(categoryTotal)
	selectivity := gateSelectivity(passRate)

	if selectivity <= 0 {
		return 0
	}

	return categoryFrac * selectivity * categoryNovelty(branches, move.category)
}

func (search *TreeSearch) reachableMoves(
	moves []Move, branches perspectives.BranchList,
) []Move {
	reachable := make([]Move, 0, len(moves))

	for _, move := range moves {
		if !search.moveReachable(move, branches) {
			continue
		}

		if !search.moveCompatible(branches, move) {
			continue
		}

		reachable = append(reachable, move)
	}

	return reachable
}

func (search *TreeSearch) sampleRolloutMove(
	moves []Move, branches perspectives.BranchList,
) Move {
	moves = search.activeMoves(moves, branches)

	weighted := make([]Move, 0, len(moves))
	weights := make([]float64, 0, len(moves))
	total := 0.0

	for _, move := range moves {
		if !search.moveReachable(move, branches) {
			continue
		}

		if !search.moveCompatible(branches, move) {
			continue
		}

		weight := search.moveWeightForBranches(move, branches)

		if weight <= 0 {
			continue
		}

		weighted = append(weighted, move)
		weights = append(weights, weight)
		total += weight
	}

	if len(weighted) == 0 {
		reachable := search.reachableMoves(moves, branches)

		if len(reachable) == 0 {
			return moves[search.rng.Intn(len(moves))]
		}

		return reachable[search.rng.Intn(len(reachable))]
	}

	pick := search.rng.Float64() * total

	for index, weight := range weights {
		pick -= weight

		if pick <= 0 {
			return weighted[index]
		}
	}

	return weighted[len(weighted)-1]
}

func (search *TreeSearch) sampleMoveIndex(
	moves []Move, branches perspectives.BranchList,
) int {
	if len(moves) <= 1 {
		return 0
	}

	candidateIndexes := search.activeMoveIndexes(moves, branches)

	total := 0.0
	weights := make([]float64, len(candidateIndexes))

	for weightIndex, moveIndex := range candidateIndexes {
		weight := search.moveWeightForBranches(moves[moveIndex], branches)

		if weight <= 0 {
			continue
		}

		weights[weightIndex] = weight
		total += weight
	}

	if total <= 0 {
		return candidateIndexes[search.rng.Intn(len(candidateIndexes))]
	}

	pick := search.rng.Float64() * total

	for weightIndex, weight := range weights {
		pick -= weight

		if pick <= 0 {
			return candidateIndexes[weightIndex]
		}
	}

	return candidateIndexes[len(candidateIndexes)-1]
}

func (search *TreeSearch) activeMoves(
	moves []Move, branches perspectives.BranchList,
) []Move {
	indexes := search.activeMoveIndexes(moves, branches)
	active := make([]Move, 0, len(indexes))

	for _, moveIndex := range indexes {
		active = append(active, moves[moveIndex])
	}

	return active
}

func (search *TreeSearch) activeMoveIndexes(
	moves []Move, branches perspectives.BranchList,
) []int {
	indexes := make([]int, 0, len(moves))

	for index := range moves {
		indexes = append(indexes, index)
	}

	if search.progress == nil ||
		!search.progress.Stagnant(search.stagnationWindow) {
		return indexes
	}

	deepening := make([]int, 0, len(indexes))

	for _, moveIndex := range indexes {
		if moves[moveIndex].observation != perspectives.ObservationNone {
			continue
		}

		if perspectives.FindEntryIndex(branches) < 0 {
			continue
		}

		deepening = append(deepening, moveIndex)
	}

	if len(deepening) > 0 {
		return deepening
	}

	return indexes
}

func (search *TreeSearch) deepeningMoves(
	moves []Move, branches perspectives.BranchList,
) []Move {
	return search.activeMoves(moves, branches)
}

func gateSelectivity(passRate float64) float64 {
	if passRate <= 0 || passRate >= 1 {
		return 0
	}

	return 4 * passRate * (1 - passRate)
}

func categoryNovelty(
	branches perspectives.BranchList, category perspectives.CategoryType,
) float64 {
	occurrences := countCategoryInBranches(branches, category)

	return 1 / float64(occurrences+1)
}

func countCategoryInBranches(
	branches perspectives.BranchList, category perspectives.CategoryType,
) int {
	count := 0

	for _, branch := range branches {
		if branch.Category == category {
			count++
		}

		count += countCategoryInBranches(
			perspectives.BranchList(branch.Branches),
			category,
		)
	}

	return count
}
