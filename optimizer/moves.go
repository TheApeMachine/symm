package optimizer

import (
	"github.com/theapemachine/symm/market/perspectives"
)

const maxBranchDepth = 2

var (
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

	searchActions = []perspectives.ActionType{
		perspectives.ActionNone,
		perspectives.ActionLimit,
		perspectives.ActionMarket,
		perspectives.ActionStopLoss,
		perspectives.ActionTakeProfit,
		perspectives.ActionSettlePosition,
	}
)

/*
Move is one MCTS expansion that appends a gated branch.
*/
type Move struct {
	depth     int
	category  perspectives.CategoryType
	condition perspectives.ConditionType
	unit      perspectives.UnitType
	quantile  float64
	action    perspectives.ActionType
}

func (search *TreeSearch) moves(
	branches perspectives.BranchList,
) []Move {
	categories := search.profile.Categories()
	moves := make([]Move, 0, len(categories)*len(searchUnits)*len(searchConditions)*len(searchQuantiles)*len(searchActions))

	for depth := 0; depth < maxBranchDepth; depth++ {
		if depth > 0 && len(branches) == 0 {
			continue
		}

		for _, category := range categories {
			for _, unit := range searchUnits {
				for _, condition := range searchConditions {
					for _, quantile := range searchQuantiles {
						for _, action := range searchActions {
							moves = append(moves, Move{
								depth:     depth,
								category:  category,
								condition: condition,
								unit:      unit,
								quantile:  quantile,
								action:    action,
							})
						}
					}
				}
			}
		}
	}

	return moves
}

func (search *TreeSearch) applyMove(
	branches perspectives.BranchList, move Move,
) perspectives.BranchList {
	branch := perspectives.Branch{
		Category:  move.category,
		Condition: move.condition,
		Unit:      move.unit,
		Value:     search.profile.Quantile(move.category, move.unit, move.quantile),
		ValueSet:  true,
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
