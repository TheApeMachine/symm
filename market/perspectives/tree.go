package perspectives

import "slices"

type Tree struct {
	Branches []Branch
}

/*
Walk traverses the tree and returns the Action at the deepest reachable leaf,
given measurements and active observation states. Branch thresholds on UnitSNR
compare against Measurement.SNR supplied by the signal.
*/
func (tree *Tree) Walk(
	measurements []Measurement,
	observations []ObservationType,
) *ActionType {
	if tree == nil {
		return nil
	}

	for _, branch := range tree.Branches {
		if action := branch.walk(measurements, observations); action != nil {
			return action
		}
	}

	return nil
}

func (branch *Branch) walk(
	measurements []Measurement,
	observations []ObservationType,
) *ActionType {
	if !branch.matches(measurements, observations) {
		return nil
	}

	for _, child := range branch.Branches {
		if action := child.walk(measurements, observations); action != nil {
			return action
		}
	}

	if branch.Action == ActionNone {
		return nil
	}

	action := branch.Action

	return &action
}

func (branch *Branch) matches(
	measurements []Measurement,
	observations []ObservationType,
) bool {
	if branch.Observation != ObservationNone {
		return slices.Contains(observations, branch.Observation)
	}

	if branch.Category == CategoryTypeNone {
		return true
	}

	measurement, ok := findMeasurement(measurements, branch.Category)

	if !ok {
		return false
	}

	switch branch.Unit {
	case UnitSNR:
		return matchesCondition(branch.Condition, measurement.SNR, branch.Value)
	case UnitConfidence:
		return matchesCondition(branch.Condition, measurement.Confidence, branch.Value)
	default:
		return true
	}
}

func findMeasurement(
	measurements []Measurement,
	category CategoryType,
) (Measurement, bool) {
	for _, measurement := range measurements {
		if measurement.Category == category {
			return measurement, true
		}
	}

	return Measurement{}, false
}

func matchesCondition(
	condition ConditionType,
	observed float64,
	threshold float64,
) bool {
	switch condition {
	case ConditionIsTrue:
		return observed != 0
	case ConditionIsFalse:
		return observed == 0
	case ConditionIsEqual:
		return observed == threshold
	case ConditionIsNotEqual:
		return observed != threshold
	case ConditionIsLessThan:
		return observed < threshold
	case ConditionIsLessThanOrEqual:
		return observed <= threshold
	case ConditionIsGreaterThanOrEqual:
		return observed >= threshold
	case ConditionIsGreaterThan:
		return observed > threshold
	default:
		return observed > threshold
	}
}
