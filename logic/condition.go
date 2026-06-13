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
	matched, _, err := condition.EvaluateIndexed(measurements, holdings, nil)

	return matched, err
}

func (condition *Condition) EvaluateIndexed(
	measurements []Measurement,
	holdings *Holdings,
	thresholdCtx *ThresholdContext,
) (bool, int, error) {
	switch condition.Type {
	case ConditionIsTrue:
		return condition.evaluateIsTrueIndexed(measurements, holdings)
	case ConditionIsFalse:
		return condition.evaluateIsFalseIndexed(measurements, holdings)
	case ConditionIsEqual:
		return condition.evaluateEqualIndexed(measurements, holdings, thresholdCtx)
	case ConditionIsNotEqual:
		matched, matchIndex, err := condition.evaluateEqualIndexed(measurements, holdings, thresholdCtx)

		if err != nil {
			return false, -1, err
		}

		return !matched, matchIndex, nil
	case ConditionIsGreaterThan:
		return condition.compareScalarsIndexed(measurements, holdings, thresholdCtx, func(left, right float64) bool {
			return left > right
		})
	case ConditionIsLessThan:
		return condition.compareScalarsIndexed(measurements, holdings, thresholdCtx, func(left, right float64) bool {
			return left < right
		})
	case ConditionIsGreaterThanOrEqual:
		return condition.compareScalarsIndexed(measurements, holdings, thresholdCtx, func(left, right float64) bool {
			return left >= right
		})
	case ConditionIsLessThanOrEqual:
		return condition.compareScalarsIndexed(measurements, holdings, thresholdCtx, func(left, right float64) bool {
			return left <= right
		})
	case ConditionIsWithin:
		return condition.evaluateWithinIndexed(measurements, holdings, thresholdCtx, true)
	case ConditionIsNotWithin:
		return condition.evaluateWithinIndexed(measurements, holdings, thresholdCtx, false)
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
			matchIndex = earliestMatchIndex(matchIndex, index)
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
	measurements []Measurement,
	holdings *Holdings,
	thresholdCtx *ThresholdContext,
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
				matchIndex = earliestMatchIndex(matchIndex, index)
			}

			continue
		}

		leftValue, ok := condition.Left.Subject.valueFrom(measurement)

		if !ok {
			continue
		}

		rightValue, rightOK, err := condition.rightScalar(measurements, thresholdCtx)

		if err != nil {
			return false, -1, err
		}

		if !rightOK {
			return false, -1, nil
		}

		if leftValue == rightValue {
			matchIndex = earliestMatchIndex(matchIndex, index)
		}
	}

	if matchIndex < 0 {
		return false, -1, nil
	}

	return true, matchIndex, nil
}

func (condition *Condition) compareScalarsIndexed(
	measurements []Measurement,
	holdings *Holdings,
	thresholdCtx *ThresholdContext,
	compare func(left float64, right float64) bool,
) (bool, int, error) {
	if condition.Left.Subject.Type == SubjectEigenmode {
		return condition.compareEigenmodeIndexed(measurements, thresholdCtx, compare)
	}

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

		rightValue, rightOK, err := condition.rightScalar(measurements, thresholdCtx)

		if err != nil {
			return false, -1, err
		}

		if !rightOK {
			return false, -1, nil
		}

		if compare(leftValue, rightValue) {
			matchIndex = earliestMatchIndex(matchIndex, index)
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
	thresholdCtx *ThresholdContext,
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

		rightValue, rightOK, err := condition.rightScalar(measurements, thresholdCtx)

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
			matchIndex = earliestMatchIndex(matchIndex, index)
		}

		if !within && !matches {
			matchIndex = earliestMatchIndex(matchIndex, index)
		}
	}

	if matchIndex < 0 {
		return false, -1, nil
	}

	return true, matchIndex, nil
}

func (condition *Condition) compareEigenmodeIndexed(
	measurements []Measurement,
	thresholdCtx *ThresholdContext,
	compare func(left float64, right float64) bool,
) (bool, int, error) {
	if condition.Left.Subject.Eigenmode == nil {
		return false, -1, nil
	}

	leftValue, ok := EigenmodeScore(measurements, condition.Left.Subject.Eigenmode.Mode)

	if !ok {
		return false, -1, nil
	}

	rightValue, rightOK, err := condition.rightScalar(measurements, thresholdCtx)

	if err != nil {
		return false, -1, err
	}

	if !rightOK {
		return false, -1, nil
	}

	if !compare(leftValue, rightValue) {
		return false, -1, nil
	}

	if len(measurements) == 0 {
		return true, -1, nil
	}

	return true, len(measurements) - 1, nil
}

func (condition *Condition) rightScalar(
	measurements []Measurement,
	thresholdCtx *ThresholdContext,
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

	if condition.Right.Subject.confidenceUsesExitBaseline {
		if thresholdCtx == nil {
			return 1.0, true, nil
		}

		return thresholdCtx.ExitConfidenceBaseline, true, nil
	}

	if condition.Right.Subject.confidenceUsesEntryBaseline {
		if thresholdCtx == nil {
			return 1.0, true, nil
		}

		return thresholdCtx.EntryConfidenceBaseline, true, nil
	}

	value, ok := condition.Right.Subject.threshold()

	return value, ok, nil
}

type BooleanType string

const (
	BooleanTypeNone     BooleanType = ""
	BooleanTypeAnd      BooleanType = "and"
	BooleanTypeOr       BooleanType = "or"
	BooleanTypeWeighted BooleanType = "weighted"
)

type ConditionGroup struct {
	Boolean    BooleanType `yaml:"boolean" json:"boolean"`
	MinScore   float64     `yaml:"min_score,omitempty" json:"min_score,omitempty"`
	Weights    []float64   `yaml:"weights,omitempty" json:"weights,omitempty"`
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
	matched, _, err := conditionGroup.EvaluateIndexed(measurements, holdings, nil)

	return matched, err
}

func (conditionGroup *ConditionGroup) EvaluateIndexed(
	measurements []Measurement,
	holdings *Holdings,
	thresholdCtx *ThresholdContext,
) (bool, int, error) {
	switch conditionGroup.Boolean {
	case BooleanTypeAnd:
		latestMatchIndex := -1

		for _, condition := range conditionGroup.Conditions {
			matched, matchIndex, err := condition.EvaluateIndexed(measurements, holdings, thresholdCtx)

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
		bestMatchIndex := -1
		matchedAny := false

		for _, condition := range conditionGroup.Conditions {
			matched, matchIndex, err := condition.EvaluateIndexed(measurements, holdings, thresholdCtx)

			if err != nil {
				return false, -1, err
			}

			if !matched {
				continue
			}

			matchedAny = true
			bestMatchIndex = earliestMatchIndex(bestMatchIndex, matchIndex)
		}

		if !matchedAny {
			return false, -1, nil
		}

		return true, bestMatchIndex, nil
	case BooleanTypeWeighted:
		return conditionGroup.evaluateWeightedIndexed(measurements, holdings, thresholdCtx)
	default:
		return false, -1, nil
	}
}

func (conditionGroup *ConditionGroup) evaluateWeightedIndexed(
	measurements []Measurement,
	holdings *Holdings,
	thresholdCtx *ThresholdContext,
) (bool, int, error) {
	if len(conditionGroup.Conditions) == 0 {
		return false, -1, nil
	}

	weights := conditionGroup.normalizedWeights()
	score := 0.0
	latestMatchIndex := -1

	for index, condition := range conditionGroup.Conditions {
		matched, matchIndex, err := condition.EvaluateIndexed(
			measurements,
			holdings,
			thresholdCtx,
		)

		if err != nil {
			return false, -1, err
		}

		partial := conditionPartialScore(matched, condition, measurements, thresholdCtx)
		score += weights[index] * partial
		latestMatchIndex = maxMatchIndex(latestMatchIndex, matchIndex)
	}

	minScore := conditionGroup.MinScore

	if minScore <= 0 {
		minScore = 1
	}

	if score < minScore {
		return false, -1, nil
	}

	return true, latestMatchIndex, nil
}

func (conditionGroup *ConditionGroup) normalizedWeights() []float64 {
	count := len(conditionGroup.Conditions)
	weights := make([]float64, count)

	if len(conditionGroup.Weights) == len(conditionGroup.Conditions) {
		copy(weights, conditionGroup.Weights)
	} else {
		for index := range weights {
			weights[index] = 1
		}
	}

	total := 0.0

	for _, weight := range weights {
		if weight > 0 {
			total += weight
		}
	}

	if total <= 0 {
		for index := range weights {
			weights[index] = 1.0 / float64(count)
		}

		return weights
	}

	for index, weight := range weights {
		if weight <= 0 {
			weights[index] = 0
			continue
		}

		weights[index] = weight / total
	}

	return weights
}

func conditionPartialScore(
	matched bool,
	condition Condition,
	measurements []Measurement,
	thresholdCtx *ThresholdContext,
) float64 {
	if matched {
		return 1
	}

	if condition.Type != ConditionIsGreaterThanOrEqual &&
		condition.Type != ConditionIsGreaterThan {
		return 0
	}

	if condition.Left.Subject.Type != SubjectConfidence {
		return 0
	}

	source := condition.Left.Subject.Source
	confidence := 0.0
	found := false

	for _, measurement := range measurements {
		if source != SourceNone && measurement.Source != source {
			continue
		}

		confidence = measurement.Confidence
		found = true
		break
	}

	if !found || confidence <= 0 {
		return 0
	}

	bar, ok, err := condition.rightScalar(measurements, thresholdCtx)

	if err != nil || !ok || bar <= 0 {
		return 0
	}

	ratio := confidence / bar

	if ratio >= 1 {
		return 1
	}

	if ratio <= 0 {
		return 0
	}

	return ratio
}
