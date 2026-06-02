package optimizer

import (
	"github.com/theapemachine/symm/market/perspectives"
)

const maxReasoningSteps = DefaultMaxReasoningSteps

var (
	searchObservations = []perspectives.ObservationType{
		perspectives.ObservationNone,
		perspectives.ObservationHolding,
		perspectives.ObservationNotHolding,
	}

	searchRegimes = []perspectives.Regime{
		perspectives.RegimeNone,
	}

	searchUnits = []perspectives.UnitType{
		perspectives.UnitSNR,
		perspectives.UnitConfidence,
	}

	searchConditions = []perspectives.ConditionType{
		perspectives.ConditionIsGreaterThanOrEqual,
		perspectives.ConditionIsLessThanOrEqual,
		perspectives.ConditionIsGreaterThan,
		perspectives.ConditionIsLessThan,
	}

	searchQuantiles = []float64{0.25, 0.5, 0.75}

	searchBranchActions = []perspectives.ActionType{
		perspectives.ActionNone,
	}

	searchEntryActions = []perspectives.ActionType{
		perspectives.ActionNone,
		perspectives.ActionLimit,
		perspectives.ActionMarket,
		perspectives.ActionIceberg,
	}

	searchExitActions = []perspectives.ActionType{
		perspectives.ActionNone,
		perspectives.ActionStopLoss,
		perspectives.ActionStopLossLimit,
		perspectives.ActionTakeProfit,
		perspectives.ActionTakeProfitLimit,
		perspectives.ActionTrailingStop,
		perspectives.ActionTrailingStopLimit,
		perspectives.ActionSettlePosition,
	}
)

/*
Move is one MCTS expansion that appends a gated branch.
*/
type Move struct {
	depth       int
	category    perspectives.CategoryType
	observation perspectives.ObservationType
	regime      perspectives.Regime
	condition   perspectives.ConditionType
	unit        perspectives.UnitType
	quantile    float64
	action      perspectives.ActionType
}

func (search *TreeSearch) moves(
	branches perspectives.BranchList,
) []Move {
	categories := search.profile.Categories()
	moveCount := len(categories) *
		len(searchObservations) *
		len(searchRegimes) *
		len(searchUnits) *
		len(searchConditions) *
		len(searchQuantiles) *
		len(searchExitActions) *
		maxReasoningSteps
	moves := make([]Move, 0, moveCount)

	for depth := 0; depth < maxReasoningSteps; depth++ {
		if depth > 0 && len(branches) == 0 {
			continue
		}

		for _, category := range categories {
			for _, observation := range searchObservations {
				actions := search.actions(observation)

				for _, regime := range searchRegimes {
					for _, unit := range searchUnits {
						for _, condition := range searchConditions {
							for _, quantile := range searchQuantiles {
								for _, action := range actions {
									moves = append(moves, Move{
										depth:       depth,
										category:    category,
										observation: observation,
										regime:      regime,
										condition:   condition,
										unit:        unit,
										quantile:    quantile,
										action:      action,
									})
								}
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
		return searchBranchActions
	}
}

func (search *TreeSearch) applyMove(
	branches perspectives.BranchList, move Move,
) perspectives.BranchList {
	branch := perspectives.Branch{
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

	if move.depth == 0 {
		next := branches.Clone()
		next = append(next, branch)

		return next
	}

	next := branches.Clone()

	if len(next) == 0 {
		return perspectives.BranchList{branch}
	}

	parent := &next[len(next)-1]
	parent.Branches = append(perspectives.BranchList(parent.Branches).Clone(), branch)

	return next
}
