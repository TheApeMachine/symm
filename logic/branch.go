package logic

import (
	"github.com/theapemachine/errnie"
)

type Branch struct {
	Branches       []*Branch       `yaml:"branches" json:"branches"`
	ConditionGroup *ConditionGroup `yaml:"condition_group" json:"condition_group"`
	Action         *Action         `yaml:"action" json:"action"`
}

func NewBranch(conditionGroup *ConditionGroup, action *Action) *Branch {
	return &Branch{
		ConditionGroup: conditionGroup,
		Action:         action,
	}
}

func (branch *Branch) Evaluate(
	measurements []Measurement, holdings *Holdings,
) (*Evaluation, error) {
	if branch.ConditionGroup == nil {
		return nil, nil
	}

	matched, matchIndex, err := branch.ConditionGroup.EvaluateIndexed(measurements, holdings, nil)

	if errnie.Error(err) != nil {
		return nil, err
	}

	if !matched {
		return nil, nil
	}

	// Slice only for children of this branch; Tree.Evaluate restarts full for siblings.
	futureTimeline := sliceTimelineAfter(measurements, matchIndex)

	if len(branch.Branches) > 0 {
		for _, child := range branch.Branches {
			evaluation, err := child.Evaluate(futureTimeline, holdings)

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

	if attribution, ok := AttributionFromConditionGroup(
		branch.ConditionGroup,
		measurements,
		holdings,
	); ok {
		stamped.ReasonSource = attribution.Source
		stamped.ReasonCategory = attribution.Category
		stamped.ExitTier = ExitTierForCategory(attribution.Category)
	}

	return &Evaluation{
		Action: &stamped,
	}, nil
}
