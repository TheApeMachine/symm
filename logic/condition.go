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
	holdings *datura.Artifact,
	left ConditionOperand,
	right ConditionOperand,
) (bool, error) {
	switch conditionType {
	case ConditionIsTrue:
		value, err := left.Resolve(measurements, holdings)
		if err != nil {
			return false, errnie.Error(errnie.Err(
				errnie.Validation,
				err.Error(),
				err,
			))
		}

		return value > 0, nil
	case ConditionIsFalse:
		value, err := left.Resolve(measurements, holdings)
		if err != nil {
			return false, errnie.Error(errnie.Err(
				errnie.Validation,
				err.Error(),
				err,
			))
		}

		return value <= 0, nil
	case ConditionIsEqual:
		comparison, err := left.Compare(measurements, holdings, right)
		if err != nil {
			return false, errnie.Error(errnie.Err(
				errnie.Validation,
				err.Error(),
				err,
			))
		}

		return comparison == 0, nil
	case ConditionIsNotEqual:
		comparison, err := left.Compare(measurements, holdings, right)
		if err != nil {
			return false, errnie.Error(errnie.Err(
				errnie.Validation,
				err.Error(),
				err,
			))
		}

		return comparison != 0, nil
	case ConditionIsGreaterThan:
		comparison, err := left.Compare(measurements, holdings, right)
		if err != nil {
			return false, errnie.Error(errnie.Err(
				errnie.Validation,
				err.Error(),
				err,
			))
		}

		return comparison > 0, nil
	case ConditionIsLessThan:
		comparison, err := left.Compare(measurements, holdings, right)
		if err != nil {
			return false, errnie.Error(errnie.Err(
				errnie.Validation,
				err.Error(),
				err,
			))
		}

		return comparison < 0, nil
	case ConditionIsGreaterThanOrEqual:
		comparison, err := left.Compare(measurements, holdings, right)
		if err != nil {
			return false, errnie.Error(errnie.Err(
				errnie.Validation,
				err.Error(),
				err,
			))
		}

		return comparison >= 0, nil
	case ConditionIsLessThanOrEqual:
		comparison, err := left.Compare(measurements, holdings, right)
		if err != nil {
			return false, errnie.Error(errnie.Err(
				errnie.Validation,
				err.Error(),
				err,
			))
		}

		return comparison <= 0, nil
	case ConditionIsWithin:
		leftValue, err := left.Resolve(measurements, holdings)

		if err != nil {
			return false, errnie.Error(errnie.Err(
				errnie.Validation,
				err.Error(),
				err,
			))
		}

		rightValue, err := right.Resolve(measurements, holdings)

		if err != nil {
			return false, errnie.Error(errnie.Err(
				errnie.Validation,
				err.Error(),
				err,
			))
		}

		return math.Abs(leftValue) <= rightValue, nil
	case ConditionIsNotWithin:
		leftValue, err := left.Resolve(measurements, holdings)

		if err != nil {
			return false, errnie.Error(errnie.Err(
				errnie.Validation,
				err.Error(),
				err,
			))
		}

		rightValue, err := right.Resolve(measurements, holdings)

		if err != nil {
			return false, errnie.Error(errnie.Err(
				errnie.Validation,
				err.Error(),
				err,
			))
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
	holdings *datura.Artifact,
) (bool, error) {
	if len(conditions) == 0 {
		return true, nil
	}

	for index := range conditions {
		matched, err := conditions[index].Evaluate(measurements, holdings)

		if err != nil {
			return false, errnie.Error(errnie.Err(
				errnie.Validation,
				err.Error(),
				err,
			))
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
	holdings *datura.Artifact,
) (bool, error) {
	matched, err := condition.Type.Evaluate(
		measurements,
		holdings,
		condition.Left,
		condition.Right,
	)

	if err != nil {
		return false, errnie.Error(errnie.Err(
			errnie.Validation,
			err.Error(),
			err,
		))
	}

	return matched, err
}

type ConditionGroup struct {
	Boolean         BooleanType      `yaml:"boolean" json:"boolean"`
	Conditions      []Condition      `yaml:"conditions" json:"conditions"`
	Groups          []ConditionGroup `yaml:"groups,omitempty" json:"groups,omitempty"`
	MinObservations int              `yaml:"min_observations,omitempty" json:"min_observations,omitempty"`
}

func (conditionGroup *ConditionGroup) Evaluate(
	measurements []*datura.Artifact,
	holdings *datura.Artifact,
) (bool, error) {
	if len(conditionGroup.Conditions) == 0 && len(conditionGroup.Groups) == 0 {
		return false, nil
	}

	boolType := conditionGroup.Boolean
	if boolType == BooleanTypeNone {
		boolType = BooleanTypeAnd
	}

	for index := range conditionGroup.Conditions {
		matched, err := conditionGroup.Conditions[index].Evaluate(measurements, holdings)

		if err != nil {
			return false, errnie.Error(errnie.Err(
				errnie.Validation,
				err.Error(),
				err,
			))
		}

		if boolType == BooleanTypeAnd && !matched {
			return false, nil
		}

		if boolType == BooleanTypeOr && matched {
			return true, nil
		}
	}

	for index := range conditionGroup.Groups {
		matched, err := conditionGroup.Groups[index].Evaluate(measurements, holdings)

		if err != nil {
			return false, errnie.Error(errnie.Err(
				errnie.Validation,
				err.Error(),
				err,
			))
		}

		if boolType == BooleanTypeAnd && !matched {
			return false, nil
		}

		if boolType == BooleanTypeOr && matched {
			return true, nil
		}
	}

	return boolType == BooleanTypeAnd, nil
}
