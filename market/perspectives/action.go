package perspectives

import "github.com/theapemachine/symm/kraken/trading"

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
