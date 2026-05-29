package perspectives

type UnitType uint8

const (
	UnitNone UnitType = iota
	UnitPercentage
	UnitPips
	UnitPoints
	UnitTicks
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
	ActionStopLoss
	ActionTakeProfit
)

type Action struct {
	ActionType ActionType
	Condition  ConditionType
	Unit       UnitType
	Value      float64
}
