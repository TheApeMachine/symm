package logic

import (
	"slices"
	"strings"
)

type WalkStepOutcome string

const (
	StepRejected WalkStepOutcome = "rejected"
	StepMatched  WalkStepOutcome = "matched"
	StepParked   WalkStepOutcome = "parked"
	StepAction   WalkStepOutcome = "action"
)

/*
WalkStep records one branch visit during a playbook descent.
*/
type WalkStep struct {
	Path    []int           `json:"path"`
	Outcome WalkStepOutcome `json:"outcome"`
	Reason  string          `json:"reason,omitempty"`
}

/*
WalkTrace is the per-evaluation audit of which branches were tried and why.
*/
type WalkTrace struct {
	Symbol     string     `json:"symbol"`
	Steps      []WalkStep `json:"steps"`
	ActivePath []int      `json:"active_path,omitempty"`
}

func (trace *WalkTrace) record(path []int, outcome WalkStepOutcome, reason string) {
	if trace == nil {
		return
	}

	trace.Steps = append(trace.Steps, WalkStep{
		Path:    slices.Clone(path),
		Outcome: outcome,
		Reason:  reason,
	})

	if outcome == StepParked {
		trace.ActivePath = slices.Clone(path)
	}
}

func actionReason(action *Action) string {
	if action == nil {
		return "action"
	}

	if action.Side != "" {
		return string(action.Type) + " " + string(action.Side)
	}

	return string(action.Type)
}

/*
EvaluationSummary compresses a walk trace into playbook audit fields.
*/
func (trace *WalkTrace) EvaluationSummary() map[string]any {
	if trace == nil {
		return map[string]any{}
	}

	summary := map[string]any{
		"symbol": trace.Symbol,
		"parked": len(trace.ActivePath) > 0,
	}

	matchedSteps := 0
	rejectedSteps := 0
	var entryBlocker string

	for _, step := range trace.Steps {
		switch step.Outcome {
		case StepMatched:
			matchedSteps++
		case StepRejected:
			rejectedSteps++

			if step.Reason == "" {
				continue
			}

			if strings.Contains(step.Reason, "held position") {
				continue
			}

			entryBlocker = step.Reason
		case StepParked:
			if step.Reason != "" {
				entryBlocker = step.Reason
			}
		case StepAction:
			entryBlocker = step.Reason
		}
	}

	summary["matched_steps"] = matchedSteps
	summary["rejected_steps"] = rejectedSteps

	if entryBlocker != "" {
		summary["entry_blocker"] = entryBlocker
	}

	if len(trace.ActivePath) > 0 {
		summary["active_path"] = trace.ActivePath
	}

	return summary
}
