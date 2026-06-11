package logic

import (
	"fmt"

	"github.com/theapemachine/errnie"
	"go.yaml.in/yaml/v3"
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

func (branch *Branch) UnmarshalYAML(node *yaml.Node) error {
	type branchFields struct {
		Branches       []*Branch       `yaml:"branches"`
		ConditionGroup *ConditionGroup `yaml:"condition_group"`
		Action         *Action         `yaml:"action"`
	}

	fields := branchFields{}

	if err := node.Decode(&fields); err != nil {
		return err
	}

	if len(fields.Branches) > 0 && fields.Action != nil {
		return fmt.Errorf("logic: branch cannot define both branches and action")
	}

	branch.Branches = fields.Branches
	branch.ConditionGroup = fields.ConditionGroup
	branch.Action = fields.Action

	return nil
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

	stamped := *branch.Action
	stamped.BranchKey = key

	return &Evaluation{
		Action: &stamped,
		Key:    key,
	}, nil
}
