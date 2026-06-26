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
WalkBranch records playbook descent steps for one branch subtree and appends
every candidate action the subtree yields to actions. The playbook only
proposes — it does not choose — so a branch with several matching children
emits all of them; the trader ranks. A child action that fires clears its
parent's sequential stage so a completed sequence does not re-arm on the next
tick. The first activePath recorded marks the principal matched route for the UI.
*/
func WalkBranch(
	branch *Branch,
	path []int,
	measurements []*datura.Artifact,
	holdings *Balances,
	steps *[]WalkStep,
	activePath *[]int,
	actions *[]*Action,
) {
	if branch == nil {
		return
	}

	now := tickTime(measurements)
	symbol := symbolFromMeasurements(measurements)

	if branch.ConditionGroup != nil {
		matched, evaluateErr := branch.ConditionGroup.Evaluate(measurements, holdings)

		if evaluateErr != nil {
			errnie.Error(errnie.Err(errnie.Validation, "logic: walk condition failed", evaluateErr))

			*steps = append(*steps, WalkStep{
				Path:    append([]int(nil), path...),
				Outcome: WalkOutcomeRejected,
				Reason:  evaluateErr.Error(),
			})

			return
		}

		step := WalkStep{
			Path:    append([]int(nil), path...),
			Outcome: WalkOutcomeRejected,
		}

		// Sequential parent: cross-tick sequencing.
		isSequential := len(branch.Branches) > 0 && branch.Action == nil

		if isSequential {
			if matched {
				branch.ensureStage().record(symbol, now, stageWindow(measurements))
				step.Outcome = WalkOutcomeMatched
			} else if branch.ensureStage().active(symbol, now) {
				step.Outcome = WalkOutcomeParked
			}

			*steps = append(*steps, step)

			// Children fire only when the parent matched on a strictly earlier
			// batch — "stage A, THEN stage B". A parent matching this very tick
			// parks; the sequence completes on a later batch.
			if !branch.ensureStage().matchedBefore(symbol, now) {
				return
			}
		} else {
			if matched {
				step.Outcome = WalkOutcomeMatched
			}

			*steps = append(*steps, step)

			if !matched {
				return
			}
		}
	}

	if branch.Action != nil {
		*steps = append(*steps, WalkStep{
			Path:    append([]int(nil), path...),
			Outcome: WalkOutcomeAction,
		})

		if len(*activePath) == 0 {
			*activePath = append([]int(nil), path...)
		}

		*actions = append(*actions, actionForSymbol(branch.Action, symbol))

		return
	}

	// Child branches are evaluated in playbook declaration order; every child
	// subtree that yields an action contributes a candidate (the playbook
	// proposes, the trader chooses).
	before := len(*actions)

	for childIndex, child := range branch.Branches {
		childPath := append(append([]int(nil), path...), childIndex)

		WalkBranch(
			child,
			childPath,
			measurements,
			holdings,
			steps,
			activePath,
			actions,
		)
	}

	// A completed sequence clears this branch's stage so it must re-arm from the
	// first stage rather than firing again on the next tick from a stale match.
	if len(*actions) > before {
		branch.ensureStage().clear(symbol)
	}
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
WalkTreeActions evaluates the playbook and returns every candidate action the
walk produced for this symbol, in playbook declaration order. The playbook only
proposes; the trader ranks and chooses. Every top-level branch and every child
subtree that matches contributes its action, so concurrent setups (the same tick
firing more than one entry, or an entry alongside a protective exit) all reach
the decider instead of being silently collapsed to the first match.
*/
func WalkTreeActions(
	symbol string,
	measurements []*datura.Artifact,
	holdings *Balances,
	branches []*Branch,
) ([]*Action, WalkTrace) {
	return walkTree(symbol, measurements, holdings, branches)
}

func walkTree(
	symbol string,
	measurements []*datura.Artifact,
	holdings *Balances,
	branches []*Branch,
) ([]*Action, WalkTrace) {
	trace := WalkTrace{
		Symbol: symbol,
		Steps:  make([]WalkStep, 0, 16),
	}

	if symbol == "" || len(measurements) == 0 || len(branches) == 0 {
		return nil, trace
	}

	activePath := make([]int, 0, 8)
	actions := make([]*Action, 0, len(branches))

	for branchIndex, branch := range branches {
		path := []int{branchIndex}

		WalkBranch(
			branch,
			path,
			measurements,
			holdings,
			&trace.Steps,
			&activePath,
			&actions,
		)
	}

	if len(activePath) > 0 {
		trace.ActivePath = append([]int(nil), activePath...)
	}

	if len(actions) == 0 {
		return nil, trace
	}

	return actions, trace
}
