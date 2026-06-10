package logic

import (
	"strconv"

	"github.com/theapemachine/errnie"
)

type Branch struct {
	Branches       []*Branch       `yaml:"branches"`
	ConditionGroup *ConditionGroup `yaml:"condition_group"`
	Action         *Action         `yaml:"action"`
}

func NewBranch(conditionGroup *ConditionGroup, action *Action) *Branch {
	return &Branch{
		ConditionGroup: conditionGroup,
		Action:         action,
	}
}

func (branch *Branch) Evaluate(
	measurements []Measurement,
	evalContext *EvalContext,
) (*Evaluation, error) {
	return branch.evaluate(measurements, evalContext, nil, nil, "")
}

func (branch *Branch) evaluate(
	measurements []Measurement,
	evalContext *EvalContext,
	stats *TreeStats,
	trace *EvalTrace,
	key string,
) (*Evaluation, error) {
	if branch.ConditionGroup == nil {
		return nil, nil
	}

	if stats != nil && key != "" {
		stats.Reach(key)
		stats.RecordConditions(key, branch.ConditionGroup, measurements, evalContext)
	}

	matched, err := branch.ConditionGroup.Evaluate(measurements, evalContext)

	if errnie.Error(err) != nil {
		return nil, err
	}

	if trace != nil && key != "" {
		trace.RecordNode(key, branch.ConditionGroup, matched, measurements, evalContext)
	}

	if stats != nil && key != "" && matched {
		stats.Hold(key)
	}

	if !matched {
		return nil, nil
	}

	for childIndex, child := range branch.Branches {
		childKey := key

		if key != "" {
			childKey = key + "/" + strconv.Itoa(childIndex)
		}

		action, err := child.evaluate(measurements, evalContext, stats, trace, childKey)

		if errnie.Error(err) != nil {
			continue
		}

		if action != nil {
			return action, nil
		}
	}

	if branch.Action == nil {
		return nil, nil
	}

	stamped := *branch.Action
	stamped.BranchKey = key

	return &Evaluation{
		Action: &stamped,
		Key:    key,
	}, nil
}
