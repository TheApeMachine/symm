package logic

import (
	"github.com/theapemachine/datura"
)

type Branch struct {
	Branches       []*Branch       `yaml:"branches" json:"branches"`
	ConditionGroup *ConditionGroup `yaml:"condition_group" json:"condition_group"`
	Action         *Action         `yaml:"action" json:"action"`
}

func (branch *Branch) Evaluate(
	measurements []*datura.Artifact,
	holdings *Balances,
) (*Action, error) {
	if branch.ConditionGroup != nil {
		matched, evaluateErr := branch.ConditionGroup.Evaluate(
			measurements,
			holdings,
		)

		if evaluateErr != nil {
			return nil, evaluateErr
		}

		if !matched {
			return nil, nil
		}
	}

	if branch.Action != nil {
		return branch.Action, nil
	}

	for _, child := range branch.Branches {
		action, err := child.Evaluate(measurements, holdings)

		if err != nil {
			return nil, err
		}

		if action != nil {
			return action, nil
		}
	}

	return nil, nil
}
