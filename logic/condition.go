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
) bool {
	return conditionOperand.Subject.Evaluate(measurement)
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

func (condition *Condition) Evaluate(measurements []Measurement) bool {
	switch condition.Type {
	case ConditionIsTrue:
		for _, measurement := range measurements {
			if condition.Left.Subject.Source != SourceNone &&
				measurement.Source != condition.Left.Subject.Source {
				continue
			}

			if condition.Left.Subject.Evaluate(measurement) {
				return true
			}
		}

		return false
	case ConditionIsFalse:
		for _, measurement := range measurements {
			if condition.Left.Subject.Source != SourceNone &&
				measurement.Source != condition.Left.Subject.Source {
				continue
			}

			return !condition.Left.Subject.Evaluate(measurement)
		}

		return false
	case ConditionIsEqual:
		return condition.evaluateEqual(measurements)
	case ConditionIsNotEqual:
		return !condition.evaluateEqual(measurements)
	case ConditionIsGreaterThan:
		return condition.compareScalars(measurements, func(left, right float64) bool {
			return left > right
		})
	case ConditionIsLessThan:
		return condition.compareScalars(measurements, func(left, right float64) bool {
			return left < right
		})
	case ConditionIsGreaterThanOrEqual:
		return condition.compareScalars(measurements, func(left, right float64) bool {
			return left >= right
		})
	case ConditionIsLessThanOrEqual:
		return condition.compareScalars(measurements, func(left, right float64) bool {
			return left <= right
		})
	case ConditionIsWithin:
		return condition.evaluateWithin(measurements, true)
	case ConditionIsNotWithin:
		return condition.evaluateWithin(measurements, false)
	default:
		return false
	}
}

func (condition *Condition) evaluateEqual(measurements []Measurement) bool {
	for _, measurement := range measurements {
		if condition.Left.Subject.Source != SourceNone &&
			measurement.Source != condition.Left.Subject.Source {
			continue
		}

		if condition.Left.Subject.isEnumerated() {
			return condition.Right.Subject.enumMatches(measurement)
		}

		leftValue, ok := condition.Left.Subject.valueFrom(measurement)

		if !ok {
			return false
		}

		rightValue, ok := condition.rightScalar(measurements)

		if !ok {
			return false
		}

		return leftValue == rightValue
	}

	return false
}

func (condition *Condition) compareScalars(
	measurements []Measurement,
	compare func(left float64, right float64) bool,
) bool {
	for _, measurement := range measurements {
		if condition.Left.Subject.Source != SourceNone &&
			measurement.Source != condition.Left.Subject.Source {
			continue
		}

		leftValue, ok := condition.Left.Subject.valueFrom(measurement)

		if !ok {
			return false
		}

		rightValue, ok := condition.rightScalar(measurements)

		if !ok {
			return false
		}

		return compare(leftValue, rightValue)
	}

	return false
}

func (condition *Condition) evaluateWithin(
	measurements []Measurement,
	within bool,
) bool {
	for _, measurement := range measurements {
		if condition.Left.Subject.Source != SourceNone &&
			measurement.Source != condition.Left.Subject.Source {
			continue
		}

		leftValue, ok := condition.Left.Subject.valueFrom(measurement)

		if !ok {
			return false
		}

		rightValue, ok := condition.rightScalar(measurements)

		if !ok {
			return false
		}

		tolerance := condition.Right.Subject.Spread

		if tolerance <= 0 {
			return false
		}

		distance := math.Abs(leftValue - rightValue)
		matches := distance <= tolerance

		if within {
			return matches
		}

		return !matches
	}

	return false
}

func (condition *Condition) rightScalar(measurements []Measurement) (float64, bool) {
	if condition.Right.Subject.Source != SourceNone {
		for _, measurement := range measurements {
			if measurement.Source == condition.Right.Subject.Source {
				return condition.Right.Subject.valueFrom(measurement)
			}
		}

		return 0, false
	}

	return condition.Right.Subject.threshold()
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

func (conditionGroup *ConditionGroup) Evaluate(measurements []Measurement) bool {
	switch conditionGroup.Boolean {
	case BooleanTypeAnd:
		for _, condition := range conditionGroup.Conditions {
			if !condition.Evaluate(measurements) {
				return false
			}
		}

		return true
	case BooleanTypeOr:
		for _, condition := range conditionGroup.Conditions {
			if condition.Evaluate(measurements) {
				return true
			}
		}

		return false
	default:
		return false
	}
}
