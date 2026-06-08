package logic

type Branch struct {
	conditionGroup *ConditionGroup
	action         *Action
}

func NewBranch(conditionGroup *ConditionGroup, action *Action) *Branch {
	return &Branch{
		conditionGroup: conditionGroup,
		action:         action,
	}
}

func (branch *Branch) Evaluate(measurements []Measurement) (*Action, error) {
	if !branch.conditionGroup.Evaluate(measurements) {
		return &Action{Type: ActionNone}, nil
	}

	return branch.action, nil
}
