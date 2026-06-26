package logic

import (
	"errors"
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

/*
Evaluate folds the conditions with the boolean operator by short-circuit
iteration: AND returns false on the first unmet condition without evaluating the
rest; OR returns true on the first met condition. An empty group is vacuously
true. Iterative, not recursive — no per-condition stack frame, and the early exit
is explicit.
*/
func (boolType BooleanType) Evaluate(
	conditions []Condition,
	measurements []*datura.Artifact,
	holdings *Balances,
) (bool, error) {
	if len(conditions) == 0 {
		return true, nil
	}

	for index := range conditions {
		matched, evaluateErr := conditions[index].Evaluate(measurements, holdings)

		if evaluateErr != nil {
			return false, evaluateErr
		}

		if boolType == BooleanTypeAnd && !matched {
			return false, nil
		}

		if boolType == BooleanTypeOr && matched {
			return true, nil
		}
	}

	// AND fell through with no unmet condition: all met. OR fell through with no
	// met condition: none met.
	return boolType == BooleanTypeAnd, nil
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

		// Absent evidence is unknown, not "false": a guard cannot pass on
		// missing data. Fail the condition closed — the playbook must see the
		// evidence to act on it.
		if errors.Is(compareErr, errUnknownMeasurement) {
			return false, nil
		}

		if compareErr != nil {
			return false, compareErr
		}

		if condition.Type == ConditionIsTrue {
			return comparison > 0, nil
		}

		return comparison < 0, nil
	}

	matched, evaluateErr := condition.Type.Evaluate(
		measurements,
		holdings,
		condition.Left,
		condition.Right,
	)

	if errors.Is(evaluateErr, errUnknownMeasurement) {
		return false, nil
	}

	return matched, evaluateErr
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
	)

	if evaluateErr != nil {
		return false, evaluateErr
	}

	return matched, nil
}
