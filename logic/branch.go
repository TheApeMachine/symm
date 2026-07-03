package logic

import "github.com/theapemachine/datura"

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
	measurements []*datura.Artifact,
	holdings *datura.Artifact,
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

			measurements[index].WithAttribute("journey.story.terminal", branch.Terminal)
			measurements[index].WithAttribute("journey.story.terminal_branch_id", branch.ID)
		}
	}

	if branch.Action != nil {
		action := *branch.Action

		if action.Symbol == "" {
			for index := range measurements {
				if measurements[index] == nil {
					continue
				}

				action.Symbol = datura.Peek[string](measurements[index], "scope")
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

			confidence := datura.Peek[float64](measurements[index], "output", "confidence")
			if confidence > action.EntryConfidence {
				action.EntryConfidence = confidence
				action.ReasonSource = SourceType(datura.Peek[string](measurements[index], "origin"))

				categoryIndex := int(datura.Peek[float64](measurements[index], "output", "value"))
				action.ReasonCategory = Categories[categoryIndex]
			}
		}

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
