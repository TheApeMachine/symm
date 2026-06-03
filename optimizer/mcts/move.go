package mcts

import "github.com/theapemachine/symm/market/perspectives"

/*
Move is one MCTS expansion that appends a gated branch.
Theoretical moves explore out-of-distribution category chains with a UCT
discount so the search maps survival responses without exhausting budget.
*/
type Move struct {
	depth       int
	category    perspectives.CategoryType
	observation perspectives.ObservationType
	regime      perspectives.Regime
	condition   perspectives.ConditionType
	unit        perspectives.UnitType
	value       float64
	action      perspectives.ActionType
	theoretical bool
	uctDiscount float64
}

func (move Move) UCTDiscount() float64 {
	return move.uctDiscount
}

func (move Move) Theoretical() bool {
	return move.theoretical
}

func (move Move) Observation() perspectives.ObservationType {
	return move.observation
}

func (move Move) Category() perspectives.CategoryType {
	return move.category
}
