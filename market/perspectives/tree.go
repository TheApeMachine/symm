package perspectives

import "slices"

type Tree struct {
	Branches []Branch
}

/*
Walk traverses the tree and returns the Action at the deepest reachable leaf the
measurements and observations support. It does not stop at the first branch that
yields an action: every branch is explored as far as the data allows, and the most
specific verdict — the one gated behind the most confirmations — wins. Depth is the
proxy for specificity because each extra level is another category or observation
the measurements had to satisfy to get there. Ties in depth resolve to the earlier
branch, so branch order still expresses priority among equally specific paths.
Branch thresholds on UnitSNR compare against Measurement.SNR supplied by the signal.
*/
func (tree *Tree) Walk(
	measurements []Measurement,
	observations []ObservationType,
) *ActionType {
	if tree == nil {
		return nil
	}

	action, _ := deepest(tree.Branches, measurements, observations)

	return action
}

/*
deepest returns the action at the deepest reachable leaf across a set of sibling
branches and the depth (in branches) at which it was found. depth is -1 when no
sibling yields an action. The first branch wins a depth tie, preserving order as
priority among equally specific paths.
*/
func deepest(
	branches []Branch,
	measurements []Measurement,
	observations []ObservationType,
) (*ActionType, int) {
	var best *ActionType
	bestDepth := -1

	for index := range branches {
		action, depth := branches[index].walk(measurements, observations)

		if action != nil && depth > bestDepth {
			best = action
			bestDepth = depth
		}
	}

	return best, bestDepth
}

/*
walk returns the action at the deepest reachable leaf under this branch and the
depth of that leaf relative to this branch — 0 when the action is the branch's own
fallback, deeper when a confirmed child supplies it. It returns (nil, -1) when the
branch does not match or exposes no action, so a non-matching branch never competes
on depth.
*/
func (branch *Branch) walk(
	measurements []Measurement,
	observations []ObservationType,
) (*ActionType, int) {
	if !branch.matches(measurements, observations) {
		return nil, -1
	}

	if action, depth := deepest(branch.Branches, measurements, observations); action != nil {
		return action, depth + 1
	}

	if branch.Action == ActionNone {
		return nil, -1
	}

	action := branch.Action

	return &action, 0
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
	case UnitSNR, UnitConfidence:
		return matchesCondition(branch.Condition, measurement.SNR, branch.Value)
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
