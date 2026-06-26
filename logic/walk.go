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

	now := tickTime(measurements)
	symbol := symbolFromMeasurements(measurements)

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

		// Sequential parent: cross-tick sequencing.
		isSequential := len(branch.Branches) > 0 && branch.Action == nil

		if isSequential {
			if matched {
				branch.ensureStage().record(symbol, now)
				step.Outcome = WalkOutcomeMatched
			} else if branch.ensureStage().active(symbol, now) {
				step.Outcome = WalkOutcomeParked
			}

			*steps = append(*steps, step)

			if !matched && !branch.ensureStage().active(symbol, now) {
				return nil
			}
		} else {
			if matched {
				step.Outcome = WalkOutcomeMatched
			}

			*steps = append(*steps, step)

			if !matched {
				return nil
			}
		}
	}

	if branch.Action != nil {
		*steps = append(*steps, WalkStep{
			Path:    append([]int(nil), path...),
			Outcome: WalkOutcomeAction,
		})
		*activePath = append([]int(nil), path...)

		return actionForSymbol(branch.Action, symbol)
	}

	// Child branches are evaluated in playbook declaration order; the first
	// child subtree that yields an action wins (no cross-branch priority field).
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
			branch.ensureStage().clear(symbol)
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
	_, trace := walkTree(symbol, measurements, holdings, branches)

	return trace
}

/*
WalkTreeAction evaluates the playbook and returns the first matching action.
Top-level branches are tried in playbook declaration order; the first branch
whose subtree yields an action wins. Within a branch, child branches are walked
in declaration order and the first matching child action wins.
*/
func WalkTreeAction(
	symbol string,
	measurements []*datura.Artifact,
	holdings *Balances,
	branches []*Branch,
) (*Action, WalkTrace) {
	return walkTree(symbol, measurements, holdings, branches)
}

func walkTree(
	symbol string,
	measurements []*datura.Artifact,
	holdings *Balances,
	branches []*Branch,
) (*Action, WalkTrace) {
	trace := WalkTrace{
		Symbol: symbol,
		Steps:  make([]WalkStep, 0, 16),
	}

	if symbol == "" || len(measurements) == 0 || len(branches) == 0 {
		return nil, trace
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

			return action, trace
		}
	}

	return nil, trace
}
