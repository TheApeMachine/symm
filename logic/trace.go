package logic

import (
	"errors"
	"strconv"
	"strings"

	"github.com/theapemachine/datura"
)

type TraceReason string

const (
	TraceReasonMatched        TraceReason = "matched"
	TraceReasonNotMatched     TraceReason = "not_matched"
	TraceReasonMissingSource  TraceReason = "missing_source"
	TraceReasonStaleSource    TraceReason = "stale_source"
	TraceReasonWrongSymbol    TraceReason = "wrong_symbol"
	TraceReasonBelowThreshold TraceReason = "below_threshold"
	TraceReasonInvalidOperand TraceReason = "invalid_operand"
	TraceReasonError          TraceReason = "error"
)

type ConditionTrace struct {
	BranchID      string        `json:"branch_id"`
	ConditionPath string        `json:"condition_path"`
	Source        SourceType    `json:"source"`
	Category      CategoryType  `json:"category"`
	Operator      ConditionType `json:"operator"`
	Result        bool          `json:"result"`
	Reason        TraceReason   `json:"reason"`
}

type EvaluationTrace struct {
	TargetSymbol      string           `json:"target_symbol"`
	BranchesEvaluated int              `json:"branches_evaluated"`
	BranchesMatched   int              `json:"branches_matched"`
	Conditions        []ConditionTrace `json:"conditions"`
}

func (tree *Tree) EvaluateTrace(
	targetSymbol string,
	measurements []*datura.Artifact,
	holdings *datura.Artifact,
	branches []*Branch,
) EvaluationTrace {
	trace := EvaluationTrace{
		TargetSymbol: targetSymbol,
		Conditions:   make([]ConditionTrace, 0),
	}
	if len(measurements) == 0 || targetSymbol == "" {
		return trace
	}

	for index, branch := range branches {
		branch.appendTrace(
			targetSymbol,
			measurements,
			holdings,
			strconv.Itoa(index),
			&trace,
		)
	}

	return trace
}

func (branch *Branch) appendTrace(
	targetSymbol string,
	measurements []*datura.Artifact,
	holdings *datura.Artifact,
	branchID string,
	trace *EvaluationTrace,
) {
	if branch == nil || trace == nil {
		return
	}

	trace.BranchesEvaluated++
	matched := true
	if branch.ConditionGroup != nil {
		conditionTraces, groupMatched := branch.ConditionGroup.Trace(
			targetSymbol,
			measurements,
			holdings,
			branchID,
			"",
		)
		matched = groupMatched
		trace.Conditions = append(trace.Conditions, conditionTraces...)
	}
	if matched {
		trace.BranchesMatched++
	}

	for index, child := range branch.Branches {
		child.appendTrace(
			targetSymbol,
			measurements,
			holdings,
			branchID+"."+strconv.Itoa(index),
			trace,
		)
	}
}

func (conditionGroup *ConditionGroup) Trace(
	targetSymbol string,
	measurements []*datura.Artifact,
	holdings *datura.Artifact,
	branchID string,
	path string,
) ([]ConditionTrace, bool) {
	if conditionGroup == nil ||
		(len(conditionGroup.Conditions) == 0 && len(conditionGroup.Groups) == 0) {
		return nil, false
	}

	boolType := conditionGroup.Boolean
	if boolType == BooleanTypeNone {
		boolType = BooleanTypeAnd
	}

	traces := make([]ConditionTrace, 0, len(conditionGroup.Conditions))
	matched := boolType == BooleanTypeAnd

	for index := range conditionGroup.Conditions {
		conditionPath := joinConditionPath(path, strconv.Itoa(index))
		conditionTrace := conditionGroup.Conditions[index].Trace(
			targetSymbol,
			measurements,
			holdings,
		)
		conditionTrace.BranchID = branchID
		conditionTrace.ConditionPath = conditionPath
		traces = append(traces, conditionTrace)

		matched = foldTraceMatch(boolType, matched, conditionTrace.Result)
	}

	for index := range conditionGroup.Groups {
		groupPath := joinConditionPath(path, "g"+strconv.Itoa(index))
		childTraces, childMatched := conditionGroup.Groups[index].Trace(
			targetSymbol,
			measurements,
			holdings,
			branchID,
			groupPath,
		)
		traces = append(traces, childTraces...)
		matched = foldTraceMatch(boolType, matched, childMatched)
	}

	return traces, matched
}

func foldTraceMatch(boolType BooleanType, current bool, next bool) bool {
	if boolType == BooleanTypeOr {
		return current || next
	}

	return current && next
}

func joinConditionPath(prefix string, next string) string {
	if prefix == "" {
		return next
	}

	return prefix + "." + next
}

func (condition *Condition) Trace(
	targetSymbol string,
	measurements []*datura.Artifact,
	holdings *datura.Artifact,
) ConditionTrace {
	trace := ConditionTrace{
		Source:   traceSource(condition),
		Category: traceCategory(condition),
		Operator: ConditionNone,
		Result:   false,
		Reason:   TraceReasonError,
	}
	if condition == nil {
		trace.Reason = TraceReasonInvalidOperand
		return trace
	}

	trace.Operator = condition.Type
	if condition.Type == ConditionIsTrue || condition.Type == ConditionIsFalse {
		value, evaluateErr := condition.Left.resolve(targetSymbol, measurements, holdings)
		if evaluateErr != nil {
			trace.Reason = traceReasonForError(evaluateErr, condition.Left, targetSymbol, measurements)
			return trace
		}

		if condition.Type == ConditionIsTrue {
			trace.Result = value > 0
		} else {
			trace.Result = value < 0
		}
		trace.Reason = traceReasonForResult(condition, trace.Result)
		return trace
	}

	matched, evaluateErr := condition.Type.Evaluate(
		targetSymbol,
		measurements,
		holdings,
		condition.Left,
		condition.Right,
	)
	if evaluateErr != nil {
		trace.Reason = traceReasonForError(evaluateErr, condition.Left, targetSymbol, measurements)
		return trace
	}

	trace.Result = matched
	trace.Reason = traceReasonForResult(condition, matched)
	return trace
}

func traceReasonForError(
	err error,
	operand ConditionOperand,
	targetSymbol string,
	measurements []*datura.Artifact,
) TraceReason {
	if errors.Is(err, errUnknownMeasurement) {
		if reason := operandEvidenceReason(operand, targetSymbol, measurements); reason != "" {
			return reason
		}

		return TraceReasonMissingSource
	}

	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(msg, "missing") || strings.Contains(msg, "unsupported operand") {
		return TraceReasonInvalidOperand
	}

	return TraceReasonError
}

func traceReasonForResult(condition *Condition, matched bool) TraceReason {
	if matched {
		return TraceReasonMatched
	}
	if condition == nil {
		return TraceReasonInvalidOperand
	}

	switch condition.Type {
	case ConditionIsGreaterThan, ConditionIsLessThan,
		ConditionIsGreaterThanOrEqual, ConditionIsLessThanOrEqual,
		ConditionIsWithin, ConditionIsNotWithin:
		return TraceReasonBelowThreshold
	default:
		if condition.Left.Type == SubjectConfidence ||
			condition.Left.Type == SubjectStrength ||
			condition.Left.Type == SubjectSurprise ||
			condition.Left.Type == SubjectElapsed {
			return TraceReasonBelowThreshold
		}

		return TraceReasonNotMatched
	}
}

func operandEvidenceReason(
	operand ConditionOperand,
	targetSymbol string,
	measurements []*datura.Artifact,
) TraceReason {
	if operand.Source == SourceNone {
		return TraceReasonMissingSource
	}

	targetSymbol = strings.ToUpper(strings.TrimSpace(targetSymbol))
	sourceSeen := false
	targetSeen := false
	staleTargetSeen := false

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		origin, err := measurement.Origin()
		if err != nil || SourceType(origin) != operand.Source {
			continue
		}
		sourceSeen = true

		scope, scopeErr := measurement.Scope()
		if scopeErr != nil || strings.ToUpper(strings.TrimSpace(scope)) != targetSymbol {
			continue
		}
		targetSeen = true

		if measurementStale(measurements, measurement) {
			staleTargetSeen = true
			continue
		}

		return ""
	}

	if targetSeen && staleTargetSeen {
		return TraceReasonStaleSource
	}
	if sourceSeen {
		return TraceReasonWrongSymbol
	}

	return TraceReasonMissingSource
}

func traceSource(condition *Condition) SourceType {
	if condition == nil {
		return SourceNone
	}
	if condition.Left.Source != SourceNone {
		return condition.Left.Source
	}

	return condition.Right.Source
}

func traceCategory(condition *Condition) CategoryType {
	if condition == nil {
		return CategoryTypeNone
	}
	if condition.Left.Category != nil {
		return condition.Left.Category.Type
	}
	if condition.Right.Category != nil {
		return condition.Right.Category.Type
	}

	return CategoryTypeNone
}
