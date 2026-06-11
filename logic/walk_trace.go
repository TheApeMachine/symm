package logic

import "slices"

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
