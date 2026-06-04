package perspectives

import (
	"fmt"

	"github.com/theapemachine/symm/kraken/trading"
)

type UnitType uint8

const (
	UnitNone UnitType = iota
	UnitPercentage
	UnitPips
	UnitPoints
	UnitTicks
	UnitTimeYears
	UnitTimeMonths
	UnitTimeWeeks
	UnitTimeDays
	UnitTimeHours
	UnitTimeMinutes
	UnitTimeSeconds
	UnitTimeMilliseconds
	UnitTimeMicroseconds
	UnitTimeNanoseconds
	UnitConfidence
	UnitSNR
)

type ConditionType uint8

const (
	ConditionNone ConditionType = iota
	ConditionIsTrue
	ConditionIsFalse
	ConditionIsEqual
	ConditionIsNotEqual
	ConditionIsGreaterThan
	ConditionIsLessThan
	ConditionIsGreaterThanOrEqual
	ConditionIsLessThanOrEqual
)

type ConditionBooleanType uint8

const (
	ConditionBooleanNone ConditionBooleanType = iota
	ConditionBooleanAnd
	ConditionBooleanOr
)

type ActionType uint8

const (
	ActionNone ActionType = iota
	ActionLimit
	ActionMarket
	ActionIceberg
	ActionStopLoss
	ActionStopLossLimit
	ActionTakeProfit
	ActionTakeProfitLimit
	ActionTrailingStop
	ActionTrailingStopLimit
	ActionSettlePosition
)

type Action struct {
	Type     ActionType
	Side     trading.Side
	Symbol   string
	Price    float64
	Quantity float64
	Offset   float64 // per-node trigger fraction (stop/take/trail); 0 = use the global default
}

/*
IsEntryAction reports whether actionType opens a position.
*/
func IsEntryAction(actionType ActionType) bool {
	switch actionType {
	case ActionLimit, ActionMarket, ActionIceberg:
		return true
	default:
		return false
	}
}

/*
IsExitAction reports whether actionType closes a position.
*/
func IsExitAction(actionType ActionType) bool {
	switch actionType {
	case ActionSettlePosition,
		ActionStopLoss,
		ActionStopLossLimit,
		ActionTakeProfit,
		ActionTakeProfitLimit,
		ActionTrailingStop,
		ActionTrailingStopLimit:
		return true
	default:
		return false
	}
}

/*
ActionFromAct builds a desk order from a reasoning decision and the current row,
carrying the per-node trigger offset through to execution. Quantity is left to the
trader, which sizes entries against the live wallet balance the exchange publishes
and settles exits against the position it currently holds.
*/
func ActionFromAct(act Act, measurement Measurement) Action {
	action := Action{
		Type:   act.Type,
		Symbol: measurement.Symbol,
		Price:  measurement.Last,
		Offset: act.Offset,
	}

	if IsEntryAction(act.Type) {
		action.Side = trading.Buy
	}

	if IsExitAction(act.Type) {
		action.Side = trading.Sell
	}

	return action
}

/*
IsMakerAction reports whether paper/Kraken would classify the fill as maker.
Only resting limit entries match kraken/paper order execution today.
*/
func IsMakerAction(actionType ActionType) bool {
	orderType, err := OrderTypeFromActionType(actionType)

	if err != nil {
		return false
	}

	return orderType == trading.Limit
}

/*
OrderTypeFromActionType maps a playbook action to the Kraken order_type string.
*/
func OrderTypeFromActionType(actionType ActionType) (trading.OrderType, error) {
	switch actionType {
	case ActionLimit, ActionIceberg:
		return trading.Limit, nil
	case ActionMarket, ActionSettlePosition:
		return trading.Market, nil
	case ActionStopLoss:
		return trading.StopLoss, nil
	case ActionStopLossLimit:
		return trading.StopLossLimit, nil
	case ActionTrailingStop:
		return trading.TrailingStop, nil
	case ActionTrailingStopLimit:
		return trading.TrailingStopLimit, nil
	case ActionTakeProfit:
		return trading.TakeProfit, nil
	case ActionTakeProfitLimit:
		return trading.TakeProfitLimit, nil
	default:
		return "", fmt.Errorf("unsupported actionType: %v", actionType)
	}
}

/*
ActionFromOrderType maps a Kraken order_type string back to a playbook action — the
inverse the paper websocket uses to tell a resting protective trigger (stop / take /
trailing) apart from an immediate fill. Returns ActionNone for unknown types.
*/
func ActionFromOrderType(orderType trading.OrderType) ActionType {
	switch orderType {
	case trading.StopLoss:
		return ActionStopLoss
	case trading.StopLossLimit:
		return ActionStopLossLimit
	case trading.TakeProfit:
		return ActionTakeProfit
	case trading.TakeProfitLimit:
		return ActionTakeProfitLimit
	case trading.TrailingStop:
		return ActionTrailingStop
	case trading.TrailingStopLimit:
		return ActionTrailingStopLimit
	default:
		return ActionNone
	}
}
