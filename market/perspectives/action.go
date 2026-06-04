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
ActionFromMeasurement builds a desk order from a tree verdict and row. Quantity is
left to the trader, which sizes entries against the live wallet balance the
exchange publishes and settles exits against the position it currently holds.
*/
func ActionFromMeasurement(
	actionType ActionType, measurement Measurement,
) Action {
	action := Action{
		Type:   actionType,
		Symbol: measurement.Symbol,
		Price:  measurement.Last,
	}

	if IsEntryAction(actionType) {
		action.Side = trading.Buy
	}

	if IsExitAction(actionType) {
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
