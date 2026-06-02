package optimizer

import (
	"github.com/theapemachine/symm/market/perspectives"
)

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
	value       float64
	action      perspectives.ActionType
}
