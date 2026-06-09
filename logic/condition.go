package logic

import "math"

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
	Subject Subject `yaml:"subject"`
}

func (conditionOperand *ConditionOperand) Evaluate(
	measurement Measurement,
	evalContext *EvalContext,
) (bool, error) {
	return conditionOperand.Subject.Evaluate(measurement, evalContext)
}

type Condition struct {
	Type  ConditionType    `yaml:"type"`
	Left  ConditionOperand `yaml:"left"`
	Right ConditionOperand `yaml:"right"`
}

func NewCondition(
	conditionType ConditionType, left ConditionOperand, right ConditionOperand,
) *Condition {
	return &Condition{
		Type: conditionType, Left: left, Right: right,
	}
}

func (condition *Condition) Evaluate(
	measurements []Measurement,
	evalContext *EvalContext,
) (bool, error) {
	switch condition.Type {
	case ConditionIsTrue:
		for _, measurement := range measurements {
			if condition.Left.Subject.Source != SourceNone &&
				measurement.Source != condition.Left.Subject.Source {
				continue
			}

			matched, err := condition.Left.Subject.Evaluate(measurement, evalContext)

			if err != nil {
				return false, err
			}

			if matched {
				return true, nil
			}
		}

		return false, nil
	case ConditionIsFalse:
		for _, measurement := range measurements {
			if condition.Left.Subject.Source != SourceNone &&
				measurement.Source != condition.Left.Subject.Source {
				continue
			}

			matched, err := condition.Left.Subject.Evaluate(measurement, evalContext)

			if err != nil {
				return false, err
			}

			return !matched, nil
		}

		return false, nil
	case ConditionIsEqual:
		return condition.evaluateEqual(measurements, evalContext)
	case ConditionIsNotEqual:
		matched, err := condition.evaluateEqual(measurements, evalContext)

		if err != nil {
			return false, err
		}

		return !matched, nil
	case ConditionIsGreaterThan:
		return condition.compareScalars(measurements, evalContext, func(left, right float64) bool {
			return left > right
		})
	case ConditionIsLessThan:
		return condition.compareScalars(measurements, evalContext, func(left, right float64) bool {
			return left < right
		})
	case ConditionIsGreaterThanOrEqual:
		return condition.compareScalars(measurements, evalContext, func(left, right float64) bool {
			return left >= right
		})
	case ConditionIsLessThanOrEqual:
		return condition.compareScalars(measurements, evalContext, func(left, right float64) bool {
			return left <= right
		})
	case ConditionIsWithin:
		return condition.evaluateWithin(measurements, evalContext, true)
	case ConditionIsNotWithin:
		return condition.evaluateWithin(measurements, evalContext, false)
	default:
		return false, nil
	}
}

func (condition *Condition) evaluateEqual(
	measurements []Measurement,
	evalContext *EvalContext,
) (bool, error) {
	for _, measurement := range measurements {
		if condition.Left.Subject.Source != SourceNone &&
			measurement.Source != condition.Left.Subject.Source {
			continue
		}

		if condition.Left.Subject.isEnumerated() {
			return condition.Right.Subject.Evaluate(measurement, evalContext)
		}

		leftValue, ok := condition.Left.Subject.valueFrom(measurement)

		if !ok {
			return false, nil
		}

		rightValue, rightOK, err := condition.rightScalar(measurements, evalContext)

		if err != nil {
			return false, err
		}

		if !rightOK {
			return false, nil
		}

		return leftValue == rightValue, nil
	}

	return false, nil
}

func (condition *Condition) compareScalars(
	measurements []Measurement,
	evalContext *EvalContext,
	compare func(left float64, right float64) bool,
) (bool, error) {
	for _, measurement := range measurements {
		if condition.Left.Subject.Source != SourceNone &&
			measurement.Source != condition.Left.Subject.Source {
			continue
		}

		leftValue, ok := condition.Left.Subject.valueFrom(measurement)

		if !ok {
			return false, nil
		}

		rightValue, rightOK, err := condition.rightScalar(measurements, evalContext)

		if err != nil {
			return false, err
		}

		if !rightOK {
			return false, nil
		}

		return compare(leftValue, rightValue), nil
	}

	return false, nil
}

func (condition *Condition) evaluateWithin(
	measurements []Measurement,
	evalContext *EvalContext,
	within bool,
) (bool, error) {
	for _, measurement := range measurements {
		if condition.Left.Subject.Source != SourceNone &&
			measurement.Source != condition.Left.Subject.Source {
			continue
		}

		leftValue, ok := condition.Left.Subject.valueFrom(measurement)

		if !ok {
			return false, nil
		}

		rightValue, rightOK, err := condition.rightScalar(measurements, evalContext)

		if err != nil {
			return false, err
		}

		if !rightOK {
			return false, nil
		}

		tolerance := condition.Right.Subject.Spread

		if tolerance <= 0 {
			return false, nil
		}

		distance := math.Abs(leftValue - rightValue)
		matches := distance <= tolerance

		if within {
			return matches, nil
		}

		return !matches, nil
	}

	return false, nil
}

func (condition *Condition) rightScalar(
	measurements []Measurement,
	evalContext *EvalContext,
) (float64, bool, error) {
	if condition.Right.Subject.Source != SourceNone {
		for _, measurement := range measurements {
			if measurement.Source == condition.Right.Subject.Source {
				value, ok := condition.Right.Subject.valueFrom(measurement)

				return value, ok, nil
			}
		}

		return 0, false, nil
	}

	if condition.Right.Subject.Type == SubjectConfidence &&
		condition.Right.Subject.Category != nil &&
		condition.Right.Subject.Category.confidenceRef != "" {
		value, err := condition.Right.Subject.Category.confidenceFloor(evalContext)

		if err != nil {
			return 0, false, err
		}

		return value, true, nil
	}

	value, ok := condition.Right.Subject.threshold()

	return value, ok, nil
}

type BooleanType uint8

const (
	BooleanTypeNone BooleanType = iota
	BooleanTypeAnd
	BooleanTypeOr
)

type ConditionGroup struct {
	Boolean    BooleanType `yaml:"boolean"`
	Conditions []Condition `yaml:"conditions"`
}

func NewConditionGroup(
	boolean BooleanType, conditions []Condition,
) *ConditionGroup {
	return &ConditionGroup{Boolean: boolean, Conditions: conditions}
}

func (conditionGroup *ConditionGroup) Evaluate(
	measurements []Measurement,
	evalContext *EvalContext,
) (bool, error) {
	switch conditionGroup.Boolean {
	case BooleanTypeAnd:
		for _, condition := range conditionGroup.Conditions {
			matched, err := condition.Evaluate(measurements, evalContext)

			if err != nil {
				return false, err
			}

			if !matched {
				return false, nil
			}
		}

		return true, nil
	case BooleanTypeOr:
		for _, condition := range conditionGroup.Conditions {
			matched, err := condition.Evaluate(measurements, evalContext)

			if err != nil {
				return false, err
			}

			if matched {
				return true, nil
			}
		}

		return false, nil
	default:
		return false, nil
	}
}
