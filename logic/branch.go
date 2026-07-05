package logic

type Branch struct {
	ID             string          `yaml:"id,omitempty"`
	Question       string          `yaml:"question,omitempty"`
	Answer         string          `yaml:"answer,omitempty"`
	Terminal       string          `yaml:"terminal,omitempty"`
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

/*
Evaluate returns every candidate action this branch subtree yields. The playbook
proposes; it does not choose, so a branch whose children each match contributes
all their actions. A nil/empty slice means the branch did not fire. Sequential
parents arm a cadence-derived stage and only let children fire on a strictly
later batch — "stage A, THEN stage B".
*/
func (branch *Branch) Evaluate(
	measurements []*Measurement,
	holdings *Holdings,
) ([]*Action, error) {
	if branch == nil {
		return nil, nil
	}

	if branch.ConditionGroup != nil {
		matched, err := branch.ConditionGroup.Evaluate(measurements, holdings)

		if err != nil {
			return nil, err
		}

		if !matched {
			return nil, nil
		}
	}

	actions := make([]*Action, 0)

	if branch.Terminal != "" {
		for index := range measurements {
			if measurements[index] == nil {
				continue
			}

			measurements[index].Story.Terminal = branch.Terminal
			measurements[index].Story.TerminalBranchID = branch.ID
		}
	}

	if branch.Action != nil {
		action := *branch.Action
		scored := false

		if action.Symbol == "" {
			for index := range measurements {
				if measurements[index] == nil {
					continue
				}

				action.Symbol = measurements[index].Symbol
				if action.Symbol != "" {
					break
				}
			}
		}

		if action.BranchKey == "" {
			action.BranchKey = branch.ID
		}

		for index := range measurements {
			if measurements[index] == nil {
				continue
			}

			score, err := measurements[index].EntryScore()
			if err != nil {
				return nil, err
			}

			if !scored || score > action.EntryScore {
				action.EntryScore = score
				action.EntryConfidence = measurements[index].Confidence
				action.ReasonSource = measurements[index].Source
				action.ReasonCategory = measurements[index].DominantCategory()
				scored = true
			}
		}

		action.Story.Status = "candidate"
		action.Story.Symbol = action.Symbol
		action.Story.Source = action.ReasonSource
		action.Story.Category = action.ReasonCategory

		actions = append(actions, &action)
	}

	for index := range branch.Branches {
		candidates, err := branch.Branches[index].Evaluate(measurements, holdings)

		if err != nil {
			return nil, err
		}

		actions = append(actions, candidates...)
	}

	return actions, nil
}
