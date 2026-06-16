package logic

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/user"
)

type Branch struct {
	Branches       []*Branch       `yaml:"branches" json:"branches"`
	ConditionGroup *ConditionGroup `yaml:"condition_group" json:"condition_group"`
	Action         *Action         `yaml:"action" json:"action"`
}

func (branch *Branch) Evaluate(
	measurements []Measurement,
	holdings *user.Balances,
) (*Action, error) {
	if branch.ConditionGroup != nil {
		matched := errnie.Does(func() (bool, error) {
			return branch.ConditionGroup.Evaluate(
				measurements,
				holdings,
			)
		}).Or(func(err error) {
			errnie.Error(errnie.Err(
				errnie.IO,
				"logic: failed to evaluate condition group",
				err,
			))
		}).Value()

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
