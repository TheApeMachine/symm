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
	measurement *Measurement,
	holdings *user.Balances,
) (*Action, error) {
	if branch.ConditionGroup == nil {
		if branch.Action != nil {
			return branch.Action, nil
		}

		return nil, nil
	}

	if errnie.Does(func() (bool, error) {
		return branch.ConditionGroup.Evaluate(
			measurement,
			holdings,
		)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.IO,
			"logic: failed to evaluate condition group",
			err,
		))
	}).Value() {
		if branch.Action != nil {
			return branch.Action, nil
		}

		return nil, nil
	}

	return nil, nil
}
