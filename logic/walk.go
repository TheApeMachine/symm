package logic

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

type WalkStepOutcome string

const (
	WalkOutcomeRejected WalkStepOutcome = "rejected"
	WalkOutcomeMatched  WalkStepOutcome = "matched"
	WalkOutcomeParked   WalkStepOutcome = "parked"
	WalkOutcomeAction   WalkStepOutcome = "action"
)

type WalkStep struct {
	Path    []int           `json:"path"`
	Outcome WalkStepOutcome `json:"outcome"`
	Reason  string          `json:"reason,omitempty"`
}

type WalkTrace struct {
	Symbol     string     `json:"symbol"`
	Steps      []WalkStep `json:"steps"`
	ActivePath []int      `json:"active_path,omitempty"`
}

/*
WalkBranch records playbook descent steps for one branch subtree.
*/
func WalkBranch(
	branch *Branch,
	path []int,
	measurements []*datura.Artifact,
	holdings *Balances,
	steps *[]WalkStep,
	activePath *[]int,
) *Action {
	if branch == nil {
		return nil
	}

	if branch.ConditionGroup != nil {
		matched, evaluateErr := branch.ConditionGroup.Evaluate(measurements, holdings)

		if evaluateErr != nil {
			errnie.Error(errnie.Err(errnie.Validation, "logic: walk condition failed", evaluateErr))

			step := WalkStep{
				Path:    append([]int(nil), path...),
				Outcome: WalkOutcomeRejected,
				Reason:  evaluateErr.Error(),
			}
			*steps = append(*steps, step)

			return nil
		}

		step := WalkStep{
			Path:    append([]int(nil), path...),
			Outcome: WalkOutcomeRejected,
		}

		if matched {
			step.Outcome = WalkOutcomeMatched
		}

		*steps = append(*steps, step)

		if !matched {
			return nil
		}
	}

	if branch.Action != nil {
		*steps = append(*steps, WalkStep{
			Path:    append([]int(nil), path...),
			Outcome: WalkOutcomeAction,
		})
		*activePath = append([]int(nil), path...)

		return branch.Action
	}

	for childIndex, child := range branch.Branches {
		childPath := append(append([]int(nil), path...), childIndex)

		if action := WalkBranch(
			child,
			childPath,
			measurements,
			holdings,
			steps,
			activePath,
		); action != nil {
			return action
		}
	}

	return nil
}

/*
WalkTree evaluates every top-level branch and returns one combined trace.
*/
func WalkTree(
	symbol string,
	measurements []*datura.Artifact,
	holdings *Balances,
	branches []*Branch,
) WalkTrace {
	trace := WalkTrace{
		Symbol: symbol,
		Steps:  make([]WalkStep, 0, 16),
	}

	if symbol == "" || len(measurements) == 0 || len(branches) == 0 {
		return trace
	}

	activePath := make([]int, 0, 8)

	for branchIndex, branch := range branches {
		path := []int{branchIndex}

		if action := WalkBranch(
			branch,
			path,
			measurements,
			holdings,
			&trace.Steps,
			&activePath,
		); action != nil {
			trace.ActivePath = append([]int(nil), activePath...)

			break
		}
	}

	return trace
}
