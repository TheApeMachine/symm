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
	return tree.WalkWithContext(measurements, observations, DecisionContext{})
}

func (tree *Tree) WalkWithContext(
	measurements []Measurement,
	observations []ObservationType,
	context DecisionContext,
) *ActionType {
	if tree == nil {
		return nil
	}

	action, _, _ := deepest(tree.Branches, measurements, observations, context)

	return action
}

/*
WalkWithTrace traverses the tree and records branch evaluations into trace.
*/
func (tree *Tree) WalkWithTrace(
	measurements []Measurement,
	observations []ObservationType,
	trace *DecisionTrace,
) *ActionType {
	return tree.WalkWithTraceContext(measurements, observations, trace, DecisionContext{})
}

func (tree *Tree) WalkWithTraceContext(
	measurements []Measurement,
	observations []ObservationType,
	trace *DecisionTrace,
	context DecisionContext,
) *ActionType {
	if tree == nil {
		return nil
	}

	action, _, steps := deepest(tree.Branches, measurements, observations, context)

	if trace != nil {
		for _, step := range steps {
			trace.RecordTraceStep(step)
		}
	}

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
	context DecisionContext,
) (*ActionType, int, []TraceStep) {
	var best *ActionType
	bestDepth := -1
	var bestSteps []TraceStep

	for index := range branches {
		action, depth, steps := branches[index].walk(measurements, observations, context)

		if action != nil && depth > bestDepth {
			best = action
			bestDepth = depth
			bestSteps = steps
		}
	}

	return best, bestDepth, bestSteps
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
	context DecisionContext,
) (*ActionType, int, []TraceStep) {
	matched, snr, threshold := branch.matchDetail(measurements, observations, context)

	if !matched {
		return nil, -1, nil
	}

	if action, depth, steps := deepest(branch.Branches, measurements, observations, context); action != nil {
		path := make([]TraceStep, 0, len(steps)+1)
		path = append(path, branch.traceStep(*action, snr, threshold, depth+1, true))
		path = append(path, steps...)

		return action, depth + 1, path
	}

	if branch.Action == ActionNone {
		return nil, -1, nil
	}

	action := branch.Action

	return &action, 0, []TraceStep{branch.traceStep(action, snr, threshold, 0, true)}
}

func (branch *Branch) traceStep(
	action ActionType,
	snr float64,
	threshold float64,
	depth int,
	matched bool,
) TraceStep {
	return TraceStep{
		Category:  branch.Category,
		Metric:    branch.Metric,
		Action:    action,
		SNR:       snr,
		Threshold: threshold,
		Condition: branch.Condition,
		Depth:     depth,
		Matched:   matched,
	}
}

func (branch *Branch) matchDetail(
	measurements []Measurement,
	observations []ObservationType,
	context DecisionContext,
) (matched bool, snr float64, threshold float64) {
	if branch.Observation != ObservationNone {
		return slices.Contains(observations, branch.Observation), 0, 0
	}

	if branch.Metric != "" {
		value, ok := context.Metric(branch.Metric)

		if !ok {
			return false, 0, branch.Value
		}

		return matchesCondition(branch.Condition, value, branch.Value), value, branch.Value
	}

	if branch.Category == CategoryTypeNone {
		return true, 0, 0
	}

	measurement, ok := findMeasurement(measurements, branch.Category)

	if !ok {
		return false, 0, branch.Value
	}

	switch branch.Unit {
	case UnitSNR, UnitConfidence:
		threshold := 0.0

		if branch.ValueSet {
			threshold = branch.Value
		}

		if !branch.ValueSet || threshold <= 0 {
			threshold = snrThreshold()
		}

		return matchesCondition(branch.Condition, measurement.SNR, threshold), measurement.SNR, threshold
	default:
		return true, measurement.SNR, branch.Value
	}
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
		return matchesCondition(branch.Condition, measurement.SNR, snrThreshold())
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
