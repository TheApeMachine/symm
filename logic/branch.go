package logic

import "github.com/theapemachine/errnie"

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
) (*Action, error) {
	if branch.ConditionGroup == nil {
		return nil, nil
	}

	matched, err := branch.ConditionGroup.Evaluate(measurements, evalContext)

	if errnie.Error(err) != nil {
		return nil, err
	}

	if !matched {
		return nil, nil
	}

	for _, child := range branch.Branches {
		action, err := child.Evaluate(measurements, evalContext)

		if errnie.Error(err) != nil {
			continue
		}

		if action != nil {
			return action, nil
		}
	}

	return branch.Action, nil
}
