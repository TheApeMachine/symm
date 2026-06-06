package reasoning

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
