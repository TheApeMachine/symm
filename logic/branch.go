package logic

import (
	"fmt"

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
	measurements []Measurement, key string, holdings *Holdings,
) (*Evaluation, error) {
	if branch.ConditionGroup == nil {
		return nil, nil
	}

	matched, matchIndex, err := branch.ConditionGroup.EvaluateIndexed(measurements, holdings)

	if errnie.Error(err) != nil {
		return nil, err
	}

	if !matched {
		return nil, nil
	}

	// Slice only for children of this branch; Tree.Evaluate restarts full for siblings.
	futureTimeline := sliceTimelineAfter(measurements, matchIndex)

	if len(branch.Branches) > 0 {
		for childIndex, child := range branch.Branches {
			childKey := fmt.Sprintf("%s.%d", key, childIndex)

			if key == "" {
				childKey = fmt.Sprintf("%d", childIndex)
			}

			evaluation, err := child.Evaluate(futureTimeline, childKey, holdings)

			if errnie.Error(err) != nil {
				return nil, err
			}

			if evaluation != nil {
				return evaluation, nil
			}
		}

		return nil, nil
	}

	if branch.Action == nil {
		return nil, nil
	}

	return &Evaluation{
		Action: branch.Action,
		Key:    key,
	}, nil
}
