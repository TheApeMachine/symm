package logic

import (
	"fmt"
	"strings"
)

func measurementBySource(measurements []Measurement, source SourceType) (Measurement, bool) {
	for _, measurement := range measurements {
		if measurement.Source == source {
			return measurement, true
		}
	}

	return Measurement{}, false
}

func (subject *Subject) label() string {
	source := string(subject.Source)

	switch subject.Type {
	case SubjectCategory:
		if subject.Category != nil {
			return fmt.Sprintf("%s · %s", source, subject.Category.Type)
		}
	case SubjectRegime:
		if subject.Regime != nil {
			return fmt.Sprintf("%s · %s", source, subject.Regime.Type)
		}
	case SubjectPosition:
		if subject.Position != nil {
			return fmt.Sprintf("%s · %s", source, subject.Position.Type)
		}
	case SubjectHolding:
		if subject.Holding != nil {
			if subject.Holding.Held {
				return "holding"
			}

			return "flat"
		}
	case SubjectConfidence:
		return fmt.Sprintf("%s · confidence ≥ %.2f", source, subject.Confidence)
	case SubjectSurprise:
		return fmt.Sprintf("%s · surprise ≥ %.2f", source, subject.Surprise)
	case SubjectStrength:
		return fmt.Sprintf("%s · strength ≥ %.2f", source, subject.Strength)
	}

	if source != "" {
		return source
	}

	return string(subject.Type)
}

func (subject *Subject) explainFalse(
	measurements []Measurement,
	holdings *Holdings,
) string {
	label := subject.label()

	switch subject.Type {
	case SubjectCategory:
		measurement, ok := measurementBySource(measurements, subject.Source)

		if !ok {
			return fmt.Sprintf("%s: no measurement in spectrum", label)
		}

		if subject.Category == nil {
			return fmt.Sprintf("%s: category unset in playbook", label)
		}

		return fmt.Sprintf(
			"%s: expected %s, got %s",
			subject.Source,
			subject.Category.Type,
			measurement.Category,
		)
	case SubjectRegime:
		measurement, ok := measurementBySource(measurements, subject.Source)

		if !ok {
			return fmt.Sprintf("%s: no measurement in spectrum", label)
		}

		if subject.Regime == nil {
			return fmt.Sprintf("%s: regime unset in playbook", label)
		}

		return fmt.Sprintf(
			"%s: expected %s, got %s",
			subject.Source,
			subject.Regime.Type,
			measurement.Regime,
		)
	case SubjectPosition:
		measurement, ok := measurementBySource(measurements, subject.Source)

		if !ok {
			return fmt.Sprintf("%s: no measurement in spectrum", label)
		}

		if subject.Position == nil {
			return fmt.Sprintf("%s: position unset in playbook", label)
		}

		return fmt.Sprintf(
			"%s: expected %s, got %s",
			subject.Source,
			subject.Position.Type,
			measurement.Position,
		)
	case SubjectHolding:
		if holdings == nil {
			return "holding gate: holdings unavailable"
		}

		symbol := ""

		for _, measurement := range measurements {
			if measurement.Symbol != "" {
				symbol = measurement.Symbol
				break
			}
		}

		held := holdings.IsHolding(symbol)

		if subject.Holding != nil && subject.Holding.Held {
			return fmt.Sprintf("requires held position, symbol is %s", holdingStateLabel(held))
		}

		return fmt.Sprintf("requires flat symbol, symbol is %s", holdingStateLabel(held))
	case SubjectConfidence, SubjectSurprise, SubjectStrength,
		SubjectPrice, SubjectVolume, SubjectSpread, SubjectElapsed:
		measurement, ok := measurementBySource(measurements, subject.Source)

		if !ok {
			return fmt.Sprintf("%s: no measurement in spectrum", label)
		}

		actual, valueOK := subject.valueFrom(measurement)

		if !valueOK {
			return fmt.Sprintf("%s: field unavailable on measurement", label)
		}

		threshold, thresholdOK := subject.threshold()

		if !thresholdOK {
			return fmt.Sprintf("%s: threshold unset in playbook", label)
		}

		return fmt.Sprintf("%s: got %.4f, need %.4f", label, actual, threshold)
	default:
		return fmt.Sprintf("%s: condition not satisfied", label)
	}
}

func holdingStateLabel(held bool) string {
	if held {
		return "held"
	}

	return "flat"
}

func (condition *Condition) ExplainFailure(
	measurements []Measurement,
	holdings *Holdings,
) string {
	matched, _, err := condition.EvaluateIndexed(measurements, holdings, nil)

	if err != nil {
		return err.Error()
	}

	if matched {
		return ""
	}

	switch condition.Type {
	case ConditionIsTrue:
		return condition.Left.Subject.explainFalse(measurements, holdings)
	case ConditionIsFalse:
		return fmt.Sprintf("¬ %s: expected false, condition is true", condition.Left.Subject.label())
	case ConditionIsGreaterThanOrEqual:
		return condition.explainCompareFailure(measurements, holdings, "≥")
	case ConditionIsGreaterThan:
		return condition.explainCompareFailure(measurements, holdings, ">")
	case ConditionIsLessThanOrEqual:
		return condition.explainCompareFailure(measurements, holdings, "≤")
	case ConditionIsLessThan:
		return condition.explainCompareFailure(measurements, holdings, "<")
	case ConditionIsEqual, ConditionIsNotEqual:
		return fmt.Sprintf("%s: equality check failed", condition.Left.Subject.label())
	default:
		return fmt.Sprintf("%s: %s not satisfied", condition.Left.Subject.label(), condition.Type)
	}
}

func (condition *Condition) explainCompareFailure(
	measurements []Measurement,
	_ *Holdings,
	operator string,
) string {
	measurement, ok := measurementBySource(measurements, condition.Left.Subject.Source)

	if !ok {
		return fmt.Sprintf("%s: no measurement in spectrum", condition.Left.Subject.label())
	}

	leftValue, leftOK := condition.Left.Subject.valueFrom(measurement)

	if !leftOK {
		return fmt.Sprintf("%s: left field unavailable", condition.Left.Subject.label())
	}

	rightValue, rightOK, err := condition.rightScalar(measurements, nil)

	if err != nil {
		return err.Error()
	}

	if !rightOK {
		rightValue, rightOK = condition.Right.Subject.valueFrom(measurement)

		if !rightOK {
			return fmt.Sprintf("%s: right field unavailable", condition.Right.Subject.label())
		}
	}

	return fmt.Sprintf(
		"%s: %.4f %s %.4f not satisfied",
		condition.Left.Subject.label(),
		leftValue,
		operator,
		rightValue,
	)
}

func (conditionGroup *ConditionGroup) ExplainFailure(
	measurements []Measurement,
	holdings *Holdings,
) string {
	if conditionGroup == nil {
		return "branch has no condition group"
	}

	switch conditionGroup.Boolean {
	case BooleanTypeAnd:
		for _, condition := range conditionGroup.Conditions {
			reason := condition.ExplainFailure(measurements, holdings)

			if reason != "" {
				return reason
			}
		}

		return "and group failed"
	case BooleanTypeOr:
		reasons := make([]string, 0, len(conditionGroup.Conditions))

		for _, condition := range conditionGroup.Conditions {
			reason := condition.ExplainFailure(measurements, holdings)

			if reason != "" {
				reasons = append(reasons, reason)
			}
		}

		if len(reasons) == 0 {
			return "or group failed"
		}

		return "none matched: " + strings.Join(reasons, "; ")
	default:
		return "unsupported boolean group"
	}
}
