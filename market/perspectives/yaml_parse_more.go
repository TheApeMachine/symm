package perspectives

import "fmt"

func parseCategory(name string) (CategoryType, error) {
	clean := cleanName(name)

	for category, categoryName := range categoryNames {
		if categoryName == clean {
			return category, nil
		}
	}

	return CategoryTypeNone, fmt.Errorf("unknown category %q", name)
}

func parseUnit(name string) (UnitType, error) {
	switch cleanName(name) {
	case "", "none":
		return UnitNone, nil
	case "percentage":
		return UnitPercentage, nil
	case "pips":
		return UnitPips, nil
	case "points":
		return UnitPoints, nil
	case "ticks":
		return UnitTicks, nil
	case "confidence":
		return UnitConfidence, nil
	case "snr":
		return UnitSNR, nil
	default:
		return UnitNone, fmt.Errorf("unknown unit %q", name)
	}
}

func parseCondition(name string) (ConditionType, error) {
	switch cleanName(name) {
	case "", ">", "gt", "greater_than":
		return ConditionIsGreaterThan, nil
	case ">=", "gte", "greater_than_or_equal":
		return ConditionIsGreaterThanOrEqual, nil
	case "<", "lt", "less_than":
		return ConditionIsLessThan, nil
	case "<=", "lte", "less_than_or_equal":
		return ConditionIsLessThanOrEqual, nil
	case "=", "==", "eq", "equal":
		return ConditionIsEqual, nil
	case "!=", "neq", "not_equal":
		return ConditionIsNotEqual, nil
	case "true", "is_true":
		return ConditionIsTrue, nil
	case "false", "is_false":
		return ConditionIsFalse, nil
	default:
		return ConditionNone, fmt.Errorf("unknown condition %q", name)
	}
}

func parseObservation(name string) (ObservationType, error) {
	switch cleanName(name) {
	case "", "none":
		return ObservationNone, nil
	case "has_started":
		return ObservationHasStarted, nil
	case "has_continued":
		return ObservationHasContinued, nil
	case "has_ended":
		return ObservationHasEnded, nil
	case "has_done_before":
		return ObservationHasDoneBefore, nil
	case "holding":
		return ObservationHolding, nil
	case "not_holding":
		return ObservationNotHolding, nil
	default:
		return ObservationNone, fmt.Errorf("unknown observation %q", name)
	}
}

func knownMetric(name string) bool {
	switch cleanName(name) {
	case MetricThesisScore,
		MetricSpreadBPS,
		MetricFeePct,
		MetricRoundTripCostBPS,
		MetricRequiredScore,
		MetricScoreCostRatio,
		MetricInPlay:
		return true
	default:
		return false
	}
}
