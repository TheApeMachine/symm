package optimizer

import (
	"github.com/theapemachine/symm/market/perspectives"
)

func valuePassesGate(
	value float64,
	threshold float64,
	condition perspectives.ConditionType,
) bool {
	switch condition {
	case perspectives.ConditionIsGreaterThanOrEqual:
		return value >= threshold
	case perspectives.ConditionIsLessThanOrEqual:
		return value <= threshold
	case perspectives.ConditionIsGreaterThan:
		return value > threshold
	case perspectives.ConditionIsLessThan:
		return value < threshold
	default:
		return false
	}
}

func (search *TreeSearch) branchFromMove(move Move) perspectives.Branch {
	return perspectives.Branch{
		Category:    move.category,
		Observation: move.observation,
		Regime:      move.regime,
		Condition:   move.condition,
		Unit:        move.unit,
		Value:       search.profile.Quantile(move.category, move.unit, move.quantile),
		ValueSet:    true,
		Action: perspectives.Action{
			Type: move.action,
		},
	}
}

func (search *TreeSearch) moveReachable(
	move Move, branches perspectives.BranchList,
) bool {
	threshold := search.profile.Quantile(move.category, move.unit, move.quantile)

	if search.profile.categoryCount(move.category) == 0 {
		return false
	}

	if search.coOccurrence != nil {
		if !search.moveChainReachable(move, branches) {
			return false
		}
	}

	return search.profile.GatePassCount(
		move.category, move.unit, move.condition, threshold,
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

func (search *TreeSearch) moveWeight(move Move) float64 {
	threshold := search.profile.Quantile(move.category, move.unit, move.quantile)
	passes := search.profile.GatePassCount(
		move.category, move.unit, move.condition, threshold,
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

	return categoryFrac * passRate
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

		weight := search.moveWeight(move)

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
