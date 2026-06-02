package perspectives

import (
	"fmt"
	"math"
	"strings"

	"github.com/theapemachine/errnie"
)

const comparisonEpsilon = 1e-9

/*
BranchContext is the market state available while evaluating a branch tree.
*/
type BranchContext struct {
	Measurements []Measurement
	Observations map[ObservationType]float64
	Regime       Regime
	Metrics      map[string]float64
}

/*
BranchEvaluator walks branches against one market state.
*/
type BranchEvaluator struct {
	context BranchContext
	err     error
}

type branchDecision struct {
	actionType ActionType
	depth      int
	found      bool
}

/*
NewBranchEvaluator creates a branch evaluator for one market state.
*/
func NewBranchEvaluator(context BranchContext) *BranchEvaluator {
	return &BranchEvaluator{context: context}
}

/*
Err returns the first invalid branch predicate encountered during evaluation.
*/
func (evaluator *BranchEvaluator) Err() error {
	return evaluator.err
}

/*
Action returns the deepest reachable branch action.
*/
func (evaluator *BranchEvaluator) Action(branches BranchList) *ActionType {
	decision := evaluator.walk(branches, 0)

	if !decision.found {
		return nil
	}

	return &decision.actionType
}

func (evaluator *BranchEvaluator) walk(
	branches BranchList, depth int,
) branchDecision {
	best := branchDecision{}

	for _, branch := range branches {
		if !evaluator.passes(branch) {
			continue
		}

		if branch.Action.Type != ActionNone && (!best.found || depth > best.depth) {
			best = branchDecision{
				actionType: branch.Action.Type,
				depth:      depth,
				found:      true,
			}
		}

		child := evaluator.walk(BranchList(branch.Branches), depth+1)

		if child.found && (!best.found || child.depth > best.depth) {
			best = child
		}
	}

	return best
}

/*
PassesBranch reports whether one branch predicate matches the current context.
*/
func (evaluator *BranchEvaluator) PassesBranch(branch Branch) bool {
	return evaluator.passes(branch)
}

func (evaluator *BranchEvaluator) passes(branch Branch) bool {
	if !evaluator.matchesRegime(branch) {
		return false
	}

	if !evaluator.matchesObservation(branch) {
		return false
	}

	if !evaluator.matchesAction(branch) {
		return false
	}

	return evaluator.matchesCategoryAndCondition(branch)
}

func (evaluator *BranchEvaluator) matchesRegime(branch Branch) bool {
	if branch.Regime == RegimeNone {
		return true
	}

	return evaluator.context.Regime == branch.Regime
}

func (evaluator *BranchEvaluator) matchesObservation(branch Branch) bool {
	if branch.Observation == ObservationNone {
		return true
	}

	value, ok := evaluator.context.Observations[branch.Observation]

	if branch.Unit != UnitNone {
		return ok
	}

	switch branch.Condition {
	case ConditionIsFalse:
		return !ok || value == 0
	case ConditionIsTrue:
		return ok && value != 0
	default:
		return ok
	}
}

func (evaluator *BranchEvaluator) matchesAction(branch Branch) bool {
	if branch.Action.Type == ActionNone {
		return true
	}

	if branch.Observation == ObservationNotHolding {
		return evaluator.isEntryAction(branch.Action.Type)
	}

	if branch.Observation == ObservationHolding {
		return evaluator.isExitAction(branch.Action.Type)
	}

	return evaluator.fail(
		"action %d requires holding or not_holding observation",
		branch.Action.Type,
	)
}

func (evaluator *BranchEvaluator) isEntryAction(actionType ActionType) bool {
	switch actionType {
	case ActionLimit, ActionMarket, ActionIceberg:
		return true
	default:
		return evaluator.fail(
			"entry observation cannot use action %d",
			actionType,
		)
	}
}

func (evaluator *BranchEvaluator) isExitAction(actionType ActionType) bool {
	switch actionType {
	case ActionStopLoss,
		ActionStopLossLimit,
		ActionTakeProfit,
		ActionTakeProfitLimit,
		ActionTrailingStop,
		ActionTrailingStopLimit,
		ActionSettlePosition:
		return true
	default:
		return evaluator.fail(
			"exit observation cannot use action %d",
			actionType,
		)
	}
}

func (evaluator *BranchEvaluator) matchesCategoryAndCondition(
	branch Branch,
) bool {
	if branch.Category == CategoryTypeNone {
		return evaluator.matchesNumericCondition(Measurement{}, branch)
	}

	for _, measurement := range evaluator.context.Measurements {
		if measurement.Category != branch.Category {
			continue
		}

		if evaluator.matchesNumericCondition(measurement, branch) {
			return true
		}
	}

	return false
}

func (evaluator *BranchEvaluator) matchesNumericCondition(
	measurement Measurement, branch Branch,
) bool {
	if branch.Unit == UnitNone && !branch.hasNumericCondition() {
		return true
	}

	if branch.Unit == UnitNone {
		return evaluator.fail(
			"numeric branch condition %d has no unit",
			branch.Condition,
		)
	}

	if branch.Condition == ConditionNone {
		return evaluator.fail(
			"branch unit %d has no condition",
			branch.Unit,
		)
	}

	value, ok := evaluator.value(measurement, branch)

	if !ok {
		return false
	}

	return evaluator.compare(value, branch.Value, branch.Condition)
}

func (branch Branch) hasNumericCondition() bool {
	switch branch.Condition {
	case ConditionIsEqual,
		ConditionIsNotEqual,
		ConditionIsGreaterThan,
		ConditionIsLessThan,
		ConditionIsGreaterThanOrEqual,
		ConditionIsLessThanOrEqual:
		return true
	default:
		return false
	}
}

func (evaluator *BranchEvaluator) value(
	measurement Measurement, branch Branch,
) (float64, bool) {
	if strings.TrimSpace(branch.Metric) != "" {
		return evaluator.metric(measurement, branch)
	}

	switch branch.Unit {
	case UnitSNR:
		if branch.Category == CategoryTypeNone {
			return 0, evaluator.fail(
				"branch unit %d requires category or metric",
				branch.Unit,
			)
		}

		return measurement.SNR, true
	case UnitConfidence:
		if branch.Category == CategoryTypeNone {
			return 0, evaluator.fail(
				"branch unit %d requires category or metric",
				branch.Unit,
			)
		}

		return measurement.Confidence, true
	case UnitPercentage,
		UnitPips,
		UnitPoints,
		UnitTicks,
		UnitTimeYears,
		UnitTimeMonths,
		UnitTimeWeeks,
		UnitTimeDays,
		UnitTimeHours,
		UnitTimeMinutes,
		UnitTimeSeconds,
		UnitTimeMilliseconds,
		UnitTimeMicroseconds,
		UnitTimeNanoseconds:
		return 0, evaluator.fail(
			"branch unit %d requires metric",
			branch.Unit,
		)
	default:
		return 0, evaluator.fail("unknown branch unit %d", branch.Unit)
	}
}

func (evaluator *BranchEvaluator) metric(
	measurement Measurement, branch Branch,
) (float64, bool) {
	name := strings.TrimSpace(branch.Metric)
	value, ok := evaluator.measurementMetric(measurement, branch, name)

	if ok {
		return value, true
	}

	value, ok = evaluator.context.Metrics[name]

	if ok {
		return value, true
	}

	return 0, evaluator.fail("branch metric %q is missing", name)
}

func (evaluator *BranchEvaluator) measurementMetric(
	measurement Measurement, branch Branch, name string,
) (float64, bool) {
	if branch.Category == CategoryTypeNone {
		return 0, false
	}

	switch name {
	case "strength":
		return measurement.Strength, true
	case "confidence":
		return measurement.Confidence, true
	case "snr":
		return measurement.SNR, true
	case "last":
		return measurement.Last, true
	default:
		return 0, false
	}
}

func (evaluator *BranchEvaluator) compare(
	left, right float64, condition ConditionType,
) bool {
	switch condition {
	case ConditionIsEqual:
		return math.Abs(left-right) < comparisonEpsilon
	case ConditionIsNotEqual:
		return math.Abs(left-right) >= comparisonEpsilon
	case ConditionIsGreaterThan:
		return left > right
	case ConditionIsLessThan:
		return left < right
	case ConditionIsGreaterThanOrEqual:
		return left >= right
	case ConditionIsLessThanOrEqual:
		return left <= right
	default:
		return evaluator.fail("unknown numeric branch condition %d", condition)
	}
}

func (evaluator *BranchEvaluator) fail(message string, args ...any) bool {
	err := errnie.Error(fmt.Errorf("perspectives: "+message, args...))

	if evaluator.err == nil {
		evaluator.err = err
	}

	return false
}
