package logic

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/user"
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
	measurement Measurement,
	holdings *user.Balances,
	right any,
	subjectType SubjectType,
) (bool, error) {
	switch conditionType {
	case ConditionIsTrue:
		return errnie.Does(func() (int, error) {
			return subjectType.Evaluate(measurement, holdings, right)
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.IO,
				"logic: failed to evaluate condition",
				err,
			))
		}).Value() > 0, nil
	case ConditionIsFalse:
		return errnie.Does(func() (int, error) {
			return subjectType.Evaluate(measurement, holdings, right)
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.IO,
				"logic: failed to evaluate condition",
				err,
			))
		}).Value() < 0, nil
	case ConditionIsEqual:
		return errnie.Does(func() (int, error) {
			return subjectType.Evaluate(measurement, holdings, right)
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.IO,
				"logic: failed to evaluate condition",
				err,
			))
		}).Value() == 0, nil
	case ConditionIsNotEqual:
		return errnie.Does(func() (int, error) {
			return subjectType.Evaluate(measurement, holdings, right)
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.IO,
				"logic: failed to evaluate condition",
				err,
			))
		}).Value() != 0, nil
	case ConditionIsGreaterThan:
		return errnie.Does(func() (int, error) {
			return subjectType.Evaluate(measurement, holdings, right)
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.IO,
				"logic: failed to evaluate condition",
				err,
			))
		}).Value() > 0, nil
	case ConditionIsLessThan:
		return errnie.Does(func() (int, error) {
			return subjectType.Evaluate(measurement, holdings, right)
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.IO,
				"logic: failed to evaluate condition",
				err,
			))
		}).Value() < 0, nil
	case ConditionIsGreaterThanOrEqual:
		return errnie.Does(func() (int, error) {
			return subjectType.Evaluate(measurement, holdings, right)
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.IO,
				"logic: failed to evaluate condition",
				err,
			))
		}).Value() >= 0, nil
	case ConditionIsLessThanOrEqual:
		return errnie.Does(func() (int, error) {
			return subjectType.Evaluate(measurement, holdings, right)
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.IO,
				"logic: failed to evaluate condition",
				err,
			))
		}).Value() <= 0, nil
	case ConditionIsWithin:
		return errnie.Does(func() (int, error) {
			return subjectType.Evaluate(measurement, holdings, right)
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.IO,
				"logic: failed to evaluate condition",
				err,
			))
		}).Value() >= 0 && errnie.Does(func() (int, error) {
			return subjectType.Evaluate(measurement, holdings, right)
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.IO,
				"logic: failed to evaluate condition",
				err,
			))
		}).Value() <= 0, nil
	case ConditionIsNotWithin:
		return errnie.Does(func() (int, error) {
			return subjectType.Evaluate(measurement, holdings, right)
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.IO,
				"logic: failed to evaluate condition",
				err,
			))
		}).Value() < 0, nil
	default:
		return false, errnie.Error(errnie.Err(
			errnie.IO,
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
	measurement Measurement,
	holdings *user.Balances,
	isTrue bool,
) (bool, error) {
	if len(conditions) == 0 {
		return isTrue, nil
	}

	condition, conditions := conditions[0], conditions[1:]

	switch boolType {
	case BooleanTypeAnd:
		if !errnie.Does(func() (bool, error) {
			return condition.Evaluate(
				measurement, holdings, condition.Left,
			)
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.IO,
				"logic: failed to evaluate condition",
				err,
			))
		}).Value() {
			return false, nil
		}

		return boolType.Evaluate(conditions, measurement, holdings, true)
	case BooleanTypeOr:
		if errnie.Does(func() (bool, error) {
			return condition.Evaluate(
				measurement, holdings, condition.Left,
			)
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.IO,
				"logic: failed to evaluate condition",
				err,
			))
		}).Value() {
			return boolType.Evaluate(conditions, measurement, holdings, true)
		}

		return boolType.Evaluate(conditions, measurement, holdings, isTrue)
	}

	return isTrue, nil
}

type Condition struct {
	Type  ConditionType `yaml:"type" json:"type"`
	Left  SubjectType   `yaml:"left" json:"left"`
	Right SubjectType   `yaml:"right" json:"right"`
}

func (condition *Condition) Evaluate(
	measurement Measurement,
	holdings *user.Balances,
	subjectType SubjectType,
) (bool, error) {
	return condition.Type.Evaluate(
		measurement,
		holdings,
		condition.Right,
		subjectType,
	)
}

type ConditionGroup struct {
	Boolean    BooleanType `yaml:"boolean" json:"boolean"`
	Conditions []Condition `yaml:"conditions" json:"conditions"`
}

func (conditionGroup *ConditionGroup) Evaluate(
	measurement Measurement,
	holdings *user.Balances,
) (bool, error) {
	if len(conditionGroup.Conditions) == 0 {
		return false, nil
	}

	if errnie.Does(func() (bool, error) {
		return conditionGroup.Boolean.Evaluate(
			conditionGroup.Conditions,
			measurement,
			holdings,
			false,
		)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.IO,
			"logic: failed to evaluate condition",
			err,
		))
	}).Value() {
		return true, nil
	}

	return false, nil
}
