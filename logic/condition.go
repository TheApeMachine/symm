package logic

import "math"

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

type ConditionOperand struct {
	Subject Subject `yaml:"subject" json:"subject"`
}

func (conditionOperand *ConditionOperand) Evaluate(
	measurement Measurement, holdings *Holdings,
) (bool, error) {
	return conditionOperand.Subject.Evaluate(measurement, holdings)
}

type Condition struct {
	Type  ConditionType    `yaml:"type" json:"type"`
	Left  ConditionOperand `yaml:"left" json:"left"`
	Right ConditionOperand `yaml:"right" json:"right"`
}

func NewCondition(
	conditionType ConditionType, left ConditionOperand, right ConditionOperand,
) *Condition {
	return &Condition{
		Type: conditionType, Left: left, Right: right,
	}
}

func (condition *Condition) Evaluate(
	measurements []Measurement, holdings *Holdings,
) (bool, error) {
	matched, _, err := condition.EvaluateIndexed(measurements, holdings)

	return matched, err
}

func (condition *Condition) EvaluateIndexed(
	measurements []Measurement, holdings *Holdings,
) (bool, int, error) {
	switch condition.Type {
	case ConditionIsTrue:
		return condition.evaluateIsTrueIndexed(measurements, holdings)
	case ConditionIsFalse:
		return condition.evaluateIsFalseIndexed(measurements, holdings)
	case ConditionIsEqual:
		return condition.evaluateEqualIndexed(measurements, holdings)
	case ConditionIsNotEqual:
		matched, matchIndex, err := condition.evaluateEqualIndexed(measurements, holdings)

		if err != nil {
			return false, -1, err
		}

		return !matched, matchIndex, nil
	case ConditionIsGreaterThan:
		return condition.compareScalarsIndexed(measurements, holdings, func(left, right float64) bool {
			return left > right
		})
	case ConditionIsLessThan:
		return condition.compareScalarsIndexed(measurements, holdings, func(left, right float64) bool {
			return left < right
		})
	case ConditionIsGreaterThanOrEqual:
		return condition.compareScalarsIndexed(measurements, holdings, func(left, right float64) bool {
			return left >= right
		})
	case ConditionIsLessThanOrEqual:
		return condition.compareScalarsIndexed(measurements, holdings, func(left, right float64) bool {
			return left <= right
		})
	case ConditionIsWithin:
		return condition.evaluateWithinIndexed(measurements, holdings, true)
	case ConditionIsNotWithin:
		return condition.evaluateWithinIndexed(measurements, holdings, false)
	default:
		return false, -1, nil
	}
}

func (condition *Condition) evaluateIsTrueIndexed(
	measurements []Measurement, holdings *Holdings,
) (bool, int, error) {
	matchIndex := -1
	matchedAny := false

	for index, measurement := range measurements {
		if condition.Left.Subject.Source != SourceNone &&
			measurement.Source != condition.Left.Subject.Source {
			continue
		}

		matched, err := condition.Left.Subject.Evaluate(measurement, holdings)

		if err != nil {
			return false, -1, err
		}

		if !matched {
			continue
		}

		matchedAny = true

		if condition.Left.Subject.anchorsTimeline() {
			matchIndex = index
		}
	}

	if !matchedAny {
		return false, -1, nil
	}

	return true, matchIndex, nil
}

func (condition *Condition) evaluateIsFalseIndexed(
	measurements []Measurement, holdings *Holdings,
) (bool, int, error) {
	for _, measurement := range measurements {
		if condition.Left.Subject.Source != SourceNone &&
			measurement.Source != condition.Left.Subject.Source {
			continue
		}

		matched, err := condition.Left.Subject.Evaluate(measurement, holdings)

		if err != nil {
			return false, -1, err
		}

		if condition.Left.Subject.Source != SourceNone {
			return !matched, -1, nil
		}

		if matched {
			return false, -1, nil
		}
	}

	return true, -1, nil
}

func (condition *Condition) evaluateEqualIndexed(
	measurements []Measurement, holdings *Holdings,
) (bool, int, error) {
	matchIndex := -1

	for index, measurement := range measurements {
		if condition.Left.Subject.Source != SourceNone &&
			measurement.Source != condition.Left.Subject.Source {
			continue
		}

		if condition.Left.Subject.isEnumerated() {
			matched, err := condition.Right.Subject.Evaluate(measurement, holdings)

			if err != nil {
				return false, -1, err
			}

			if matched {
				matchIndex = index
			}

			continue
		}

		leftValue, ok := condition.Left.Subject.valueFrom(measurement)

		if !ok {
			continue
		}

		rightValue, rightOK, err := condition.rightScalar(measurements)

		if err != nil {
			return false, -1, err
		}

		if !rightOK {
			return false, -1, nil
		}

		if leftValue == rightValue {
			matchIndex = index
		}
	}

	if matchIndex < 0 {
		return false, -1, nil
	}

	return true, matchIndex, nil
}

func (condition *Condition) compareScalarsIndexed(
	measurements []Measurement,
	_ *Holdings,
	compare func(left float64, right float64) bool,
) (bool, int, error) {
	matchIndex := -1

	for index, measurement := range measurements {
		if condition.Left.Subject.Source != SourceNone &&
			measurement.Source != condition.Left.Subject.Source {
			continue
		}

		leftValue, ok := condition.Left.Subject.valueFrom(measurement)

		if !ok {
			continue
		}

		rightValue, rightOK, err := condition.rightScalar(measurements)

		if err != nil {
			return false, -1, err
		}

		if !rightOK {
			return false, -1, nil
		}

		if compare(leftValue, rightValue) {
			matchIndex = index
		}
	}

	if matchIndex < 0 {
		return false, -1, nil
	}

	return true, matchIndex, nil
}

func (condition *Condition) evaluateWithinIndexed(
	measurements []Measurement,
	_ *Holdings,
	within bool,
) (bool, int, error) {
	matchIndex := -1

	for index, measurement := range measurements {
		if condition.Left.Subject.Source != SourceNone &&
			measurement.Source != condition.Left.Subject.Source {
			continue
		}

		leftValue, ok := condition.Left.Subject.valueFrom(measurement)

		if !ok {
			continue
		}

		rightValue, rightOK, err := condition.rightScalar(measurements)

		if err != nil {
			return false, -1, err
		}

		if !rightOK {
			return false, -1, nil
		}

		tolerance := condition.Right.Subject.Spread

		if tolerance <= 0 {
			return false, -1, nil
		}

		distance := math.Abs(leftValue - rightValue)
		matches := distance <= tolerance

		if within && matches {
			matchIndex = index
		}

		if !within && !matches {
			matchIndex = index
		}
	}

	if matchIndex < 0 {
		return false, -1, nil
	}

	return true, matchIndex, nil
}

func (condition *Condition) rightScalar(
	measurements []Measurement,
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

	value, ok := condition.Right.Subject.threshold()

	return value, ok, nil
}

type BooleanType string

const (
	BooleanTypeNone BooleanType = ""
	BooleanTypeAnd  BooleanType = "and"
	BooleanTypeOr   BooleanType = "or"
)

type ConditionGroup struct {
	Boolean    BooleanType `yaml:"boolean" json:"boolean"`
	Conditions []Condition `yaml:"conditions" json:"conditions"`
}

func NewConditionGroup(
	boolean BooleanType, conditions []Condition,
) *ConditionGroup {
	return &ConditionGroup{Boolean: boolean, Conditions: conditions}
}

func (conditionGroup *ConditionGroup) Evaluate(
	measurements []Measurement, holdings *Holdings,
) (bool, error) {
	matched, _, err := conditionGroup.EvaluateIndexed(measurements, holdings)

	return matched, err
}

func (conditionGroup *ConditionGroup) EvaluateIndexed(
	measurements []Measurement, holdings *Holdings,
) (bool, int, error) {
	switch conditionGroup.Boolean {
	case BooleanTypeAnd:
		latestMatchIndex := -1

		for _, condition := range conditionGroup.Conditions {
			matched, matchIndex, err := condition.EvaluateIndexed(measurements, holdings)

			if err != nil {
				return false, -1, err
			}

			if !matched {
				return false, -1, nil
			}

			latestMatchIndex = maxMatchIndex(latestMatchIndex, matchIndex)
		}

		return true, latestMatchIndex, nil
	case BooleanTypeOr:
		for _, condition := range conditionGroup.Conditions {
			matched, matchIndex, err := condition.EvaluateIndexed(measurements, holdings)

			if err != nil {
				return false, -1, err
			}

			if matched {
				return true, matchIndex, nil
			}
		}

		return false, -1, nil
	default:
		return false, -1, nil
	}
}
