package logic

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
	ConditionIsWithin
	ConditionIsNotWithin
)

type ConditionOperand struct {
	Subject Subject
}

func (conditionOperand *ConditionOperand) Evaluate(
	measurement Measurement,
) bool {
	return conditionOperand.Subject.Evaluate(measurement)
}

type Condition struct {
	Type  ConditionType
	Left  ConditionOperand
	Right ConditionOperand
}

func NewCondition(
	conditionType ConditionType, left ConditionOperand, right ConditionOperand,
) *Condition {
	return &Condition{
		Type: conditionType, Left: left, Right: right,
	}
}

func (condition *Condition) Evaluate(left, right Measurement) bool {
	switch condition.Type {
	case ConditionIsTrue:
		return condition.Left.Evaluate(left) && condition.Right.Evaluate(right)
	case ConditionIsFalse:
		return false
	}

	return false
}

type BooleanType uint8

const (
	BooleanTypeNone BooleanType = iota
	BooleanTypeAnd
	BooleanTypeOr
)

type ConditionGroup struct {
	Boolean    BooleanType
	Conditions []Condition
}

func NewConditionGroup(
	boolean BooleanType, conditions []Condition,
) *ConditionGroup {
	return &ConditionGroup{Boolean: boolean, Conditions: conditions}
}

func (conditionGroup *ConditionGroup) Evaluate(measurements []Measurement) bool {
	switch conditionGroup.Boolean {
	case BooleanTypeAnd:
		for _, condition := range conditionGroup.Conditions {
			if !condition.Evaluate(measurements[0], measurements[1]) {
				return false
			}
		}

		return true
	case BooleanTypeOr:
		for _, condition := range conditionGroup.Conditions {
			if condition.Evaluate(measurements[0], measurements[1]) {
				return true
			}
		}

		return false
	default:
		return false
	}
}
