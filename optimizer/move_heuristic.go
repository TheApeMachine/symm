package optimizer

import (
	"github.com/theapemachine/symm/market/perspectives"
)

/*
GatePassCount returns rows where category matches and the gate would fire.
*/
func (profile *Profile) GatePassCount(
	category perspectives.CategoryType,
	unit perspectives.UnitType,
	condition perspectives.ConditionType,
	threshold float64,
) int {
	count := 0

	for _, row := range profile.rows {
		if row.Category != category {
			continue
		}

		value, ok := profile.value(row, unit)

		if !ok {
			continue
		}

		if valuePassesGate(value, threshold, condition) {
			count++
		}
	}

	return count
}

func (profile *Profile) categoryCount(category perspectives.CategoryType) int {
	count := 0

	for _, row := range profile.rows {
		if row.Category == category {
			count++
		}
	}

	return count
}

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

func (search *TreeSearch) moveReachable(move Move) bool {
	threshold := search.profile.Quantile(move.category, move.unit, move.quantile)

	if search.profile.categoryCount(move.category) == 0 {
		return false
	}

	return search.profile.GatePassCount(
		move.category, move.unit, move.condition, threshold,
	) > 0
}

func (search *TreeSearch) moveCompatible(
	branches perspectives.BranchList, move Move,
) bool {
	if len(branches) == 0 {
		return true
	}

	parent := branches[len(branches)-1]

	return isBranchCompatible(parent, search.branchFromMove(move))
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
		if !search.moveReachable(move) {
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
		if !search.moveReachable(move) {
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
