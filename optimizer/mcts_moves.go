package optimizer

import (
	"github.com/theapemachine/symm/market/perspectives"
)

func (search *TreeSearch) allMoves() []Move {
	return search.cachedMoves
}

func (search *TreeSearch) generateAllMoves() []Move {
	categories := search.profile.Categories()
	moves := make([]Move, 0)

	for _, category := range categories {
		for _, observation := range searchObservations {
			actions := search.actions(observation)

			for _, regime := range searchRegimes {
				for _, unit := range searchUnits {
					values := search.profile.AdaptiveValues(
						category,
						unit,
						search.maxThresholds,
					)

					for _, condition := range searchConditions {
						for _, value := range values {
							for _, action := range actions {
								moves = append(moves, Move{
									category:    category,
									observation: observation,
									regime:      regime,
									condition:   condition,
									unit:        unit,
									value:       value,
									action:      action,
								})
							}
						}
					}
				}
			}
		}
	}

	return moves
}

func (search *TreeSearch) actions(
	observation perspectives.ObservationType,
) []perspectives.ActionType {
	switch observation {
	case perspectives.ObservationNotHolding:
		return searchEntryActions
	case perspectives.ObservationHolding:
		return searchExitActions
	default:
		return []perspectives.ActionType{perspectives.ActionNone}
	}
}

func (search *TreeSearch) applyMove(
	branches perspectives.BranchList, move Move,
) perspectives.BranchList {
	branch := search.branchFromMove(move)

	if len(branches) == 0 {
		return perspectives.BranchList{branch}
	}

	switch move.observation {
	case perspectives.ObservationNone:
		if perspectives.FindEntryIndex(branches) >= 0 {
			nested, ok := nestGateUnderEntry(branches, branch)

			if ok {
				return nested
			}
		}

		return branches.Clone()
	case perspectives.ObservationNotHolding:
		return appendEntryPathSibling(branches, branch)
	case perspectives.ObservationHolding:
		return appendExitSibling(branches, branch)
	default:
		return append(branches.Clone(), branch)
	}
}
