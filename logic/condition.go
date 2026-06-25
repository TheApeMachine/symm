package logic

import (
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

type ConditionType string

const (
	ConditionNone                 ConditionType = ""
	ConditionIsTrue               ConditionType = "is_true"
	ConditionIsFalse              ConditionType = "is_false"
	ConditionIsEqual              ConditionType = "is_equal"
	ConditionIsNotEqual           ConditionType = "is_not_equal"
	ConditionIsGreaterThan        ConditionType = "is_greater_than"
	ConditionIsLessThan           ConditionType = "is_less_than"
	ConditionIsGreaterThanOrEqual ConditionType = "is_greater_than_or_equal"
	ConditionIsLessThanOrEqual    ConditionType = "is_less_than_or_equal"
	ConditionIsWithin             ConditionType = "is_within"
	ConditionIsNotWithin          ConditionType = "is_not_within"
)

func (conditionType ConditionType) Evaluate(
	measurements []*datura.Artifact,
	holdings *Balances,
	left ConditionOperand,
	right ConditionOperand,
) (bool, error) {
	comparison, compareErr := left.Compare(measurements, holdings, right)

	if compareErr != nil {
		return false, compareErr
	}

	switch conditionType {
	case ConditionIsTrue:
		return comparison > 0, nil
	case ConditionIsFalse:
		return comparison < 0, nil
	case ConditionIsEqual:
		return comparison == 0, nil
	case ConditionIsNotEqual:
		return comparison != 0, nil
	case ConditionIsGreaterThan:
		return comparison > 0, nil
	case ConditionIsLessThan:
		return comparison < 0, nil
	case ConditionIsGreaterThanOrEqual:
		return comparison >= 0, nil
	case ConditionIsLessThanOrEqual:
		return comparison <= 0, nil
	case ConditionIsWithin:
		leftValue, leftErr := left.resolve(measurements, holdings)

		if leftErr != nil {
			return false, leftErr
		}

		rightValue, rightErr := right.resolve(measurements, holdings)

		if rightErr != nil {
			return false, rightErr
		}

		return math.Abs(leftValue) <= rightValue, nil
	case ConditionIsNotWithin:
		leftValue, leftErr := left.resolve(measurements, holdings)

		if leftErr != nil {
			return false, leftErr
		}

		rightValue, rightErr := right.resolve(measurements, holdings)

		if rightErr != nil {
			return false, rightErr
		}

		return math.Abs(leftValue) > rightValue, nil
	default:
		return false, errnie.Error(errnie.Err(
			errnie.Validation,
			"logic: invalid condition type",
			nil,
		))
	}
}

type BooleanType string

const (
	BooleanTypeNone BooleanType = ""
	BooleanTypeAnd  BooleanType = "and"
	BooleanTypeOr   BooleanType = "or"
)

func (boolType BooleanType) Evaluate(
	conditions []Condition,
	measurements []*datura.Artifact,
	holdings *Balances,
	isTrue bool,
) (bool, error) {
	if len(conditions) == 0 {
		return isTrue, nil
	}

	condition, conditions := conditions[0], conditions[1:]

	switch boolType {
	case BooleanTypeAnd:
		matched, evaluateErr := condition.Evaluate(measurements, holdings)

		if evaluateErr != nil {
			return false, evaluateErr
		}

		if !matched {
			return false, nil
		}

		return boolType.Evaluate(conditions, measurements, holdings, true)
	case BooleanTypeOr:
		matched, evaluateErr := condition.Evaluate(measurements, holdings)

		if evaluateErr != nil {
			return false, evaluateErr
		}

		if matched {
			return boolType.Evaluate(conditions, measurements, holdings, true)
		}

		return boolType.Evaluate(conditions, measurements, holdings, isTrue)
	}

	return isTrue, nil
}

type Condition struct {
	Type  ConditionType    `yaml:"type" json:"type"`
	Left  ConditionOperand `yaml:"left" json:"left"`
	Right ConditionOperand `yaml:"right,omitempty" json:"right,omitempty"`
}

func (condition *Condition) Evaluate(
	measurements []*datura.Artifact,
	holdings *Balances,
) (bool, error) {
	if condition.Type == ConditionIsTrue || condition.Type == ConditionIsFalse {
		comparison, compareErr := condition.Left.resolve(measurements, holdings)

		if compareErr != nil {
			return false, compareErr
		}

		if condition.Type == ConditionIsTrue {
			return comparison > 0, nil
		}

		return comparison < 0, nil
	}

	return condition.Type.Evaluate(
		measurements,
		holdings,
		condition.Left,
		condition.Right,
	)
}

type ConditionGroup struct {
	Boolean    BooleanType `yaml:"boolean" json:"boolean"`
	Conditions []Condition `yaml:"conditions" json:"conditions"`
}

func (conditionGroup *ConditionGroup) Evaluate(
	measurements []*datura.Artifact,
	holdings *Balances,
) (bool, error) {
	if len(conditionGroup.Conditions) == 0 {
		return false, nil
	}

	matched, evaluateErr := conditionGroup.Boolean.Evaluate(
		conditionGroup.Conditions,
		measurements,
		holdings,
		false,
	)

	if evaluateErr != nil {
		return false, evaluateErr
	}

	return matched, nil
}
