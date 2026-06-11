package logic

import (
	"errors"
	"slices"

	"github.com/theapemachine/errnie"
)

/*
WalkState stores a paused playbook descent waiting for more timeline data.
*/
type WalkState struct {
	BranchPath []int `json:"branch_path"`
	MatchIndex int   `json:"match_index"`
}

/*
EvaluateContinuing walks the playbook from state, or from the root when state is nil.
It parks when a matched branch needs timeline data that is not available yet.
Pass trace to collect branch visit outcomes for the dashboard decision tree.
*/
func (tree *Tree) EvaluateContinuing(
	measurements []Measurement,
	holdings *Holdings,
	state *WalkState,
	trace *WalkTrace,
) (*Evaluation, *WalkState, error) {
	if tree == nil {
		return nil, nil, errnie.Error(errors.New("logic: tree is nil"))
	}

	if state != nil && len(state.BranchPath) > 0 {
		return tree.continueWalk(measurements, holdings, state, trace)
	}

	for branchIndex, branch := range tree.Branches {
		path := []int{branchIndex}

		evaluation, nextState, err := branch.evaluateResumable(
			measurements,
			holdings,
			path,
			trace,
		)

		if errnie.Error(err) != nil {
			return nil, nil, err
		}

		if evaluation != nil {
			return evaluation, nil, nil
		}

		if nextState != nil {
			return nil, nextState, nil
		}
	}

	return nil, nil, nil
}

func (tree *Tree) continueWalk(
	measurements []Measurement,
	holdings *Holdings,
	state *WalkState,
	trace *WalkTrace,
) (*Evaluation, *WalkState, error) {
	branch, err := tree.resolveBranch(state.BranchPath)

	if errnie.Error(err) != nil {
		return nil, nil, err
	}

	futureTimeline := sliceTimelineAfter(measurements, state.MatchIndex)

	if len(futureTimeline) == 0 {
		if trace != nil {
			trace.record(state.BranchPath, StepParked, "waiting for later timeline tick")
		}

		return nil, state, nil
	}

	for childIndex, child := range branch.Branches {
		childPath := append(slices.Clone(state.BranchPath), childIndex)

		evaluation, nextState, err := child.evaluateResumable(
			futureTimeline,
			holdings,
			childPath,
			trace,
		)

		if errnie.Error(err) != nil {
			return nil, nil, err
		}

		if evaluation != nil {
			return evaluation, nil, nil
		}

		if nextState != nil {
			return nil, nextState, nil
		}
	}

	return nil, nil, nil
}

func (tree *Tree) resolveBranch(path []int) (*Branch, error) {
	if len(path) == 0 {
		return nil, errnie.Error(errors.New("logic: empty branch path"))
	}

	if path[0] < 0 || path[0] >= len(tree.Branches) {
		return nil, errnie.Error(errors.New("logic: branch path out of range"))
	}

	branch := tree.Branches[path[0]]

	for _, branchIndex := range path[1:] {
		if branch == nil {
			return nil, errnie.Error(errors.New("logic: branch path terminates early"))
		}

		if branchIndex < 0 || branchIndex >= len(branch.Branches) {
			return nil, errnie.Error(errors.New("logic: branch path out of range"))
		}

		branch = branch.Branches[branchIndex]
	}

	if branch == nil {
		return nil, errnie.Error(errors.New("logic: branch path resolves to nil"))
	}

	return branch, nil
}

func (branch *Branch) evaluateResumable(
	measurements []Measurement,
	holdings *Holdings,
	path []int,
	trace *WalkTrace,
) (*Evaluation, *WalkState, error) {
	if branch.ConditionGroup == nil {
		return nil, nil, nil
	}

	matched, matchIndex, err := branch.ConditionGroup.EvaluateIndexed(measurements, holdings)

	if errnie.Error(err) != nil {
		return nil, nil, err
	}

	if !matched {
		if trace != nil {
			trace.record(path, StepRejected, branch.ConditionGroup.ExplainFailure(measurements, holdings))
		}

		return nil, nil, nil
	}

	if len(branch.Branches) == 0 {
		if branch.Action == nil {
			return nil, nil, nil
		}

		if trace != nil {
			trace.record(path, StepAction, actionReason(branch.Action))
		}

		return &Evaluation{Action: branch.Action}, nil, nil
	}

	if trace != nil {
		trace.record(path, StepMatched, "")
	}

	futureTimeline := sliceTimelineAfter(measurements, matchIndex)

	if len(futureTimeline) == 0 {
		if trace != nil {
			trace.record(path, StepParked, "waiting for later timeline tick after parent match")
		}

		return nil, &WalkState{
			BranchPath: slices.Clone(path),
			MatchIndex: matchIndex,
		}, nil
	}

	for childIndex, child := range branch.Branches {
		childPath := append(slices.Clone(path), childIndex)

		evaluation, nextState, err := child.evaluateResumable(
			futureTimeline,
			holdings,
			childPath,
			trace,
		)

		if errnie.Error(err) != nil {
			return nil, nil, err
		}

		if evaluation != nil {
			return evaluation, nil, nil
		}

		if nextState != nil {
			return nil, nextState, nil
		}
	}

	return nil, nil, nil
}
