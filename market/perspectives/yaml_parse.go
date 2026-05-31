package perspectives

import "fmt"

func buildPrimitiveBranch(spec BranchSpec) (Branch, error) {
	branch := Branch{
		Condition: ConditionIsGreaterThan,
	}

	if spec.Condition != "" {
		condition, err := parseCondition(spec.Condition)

		if err != nil {
			return Branch{}, err
		}

		branch.Condition = condition
	}

	if spec.Value != nil {
		branch.Value = *spec.Value
		branch.ValueSet = true
	}

	if spec.Category != "" {
		category, err := parseCategory(spec.Category)

		if err != nil {
			return Branch{}, err
		}

		branch.Category = category
		branch.Unit = UnitSNR
	}

	if spec.Unit != "" {
		unit, err := parseUnit(spec.Unit)

		if err != nil {
			return Branch{}, err
		}

		branch.Unit = unit
	}

	if spec.Observation != "" {
		observation, err := parseObservation(spec.Observation)

		if err != nil {
			return Branch{}, err
		}

		branch.Observation = observation
	}

	if spec.Metric != "" {
		if !knownMetric(spec.Metric) {
			return Branch{}, fmt.Errorf("unknown metric %q", spec.Metric)
		}

		branch.Metric = cleanName(spec.Metric)
	}

	if spec.Action != "" {
		action, err := parseAction(spec.Action)

		if err != nil {
			return Branch{}, err
		}

		branch.Action = action
	}

	if branch.Category == CategoryTypeNone &&
		branch.Observation == ObservationNone &&
		branch.Metric == "" &&
		len(spec.Branches) == 0 {
		return Branch{}, fmt.Errorf("branch requires category, observation, metric, or branches")
	}

	return branch, nil
}

func parsePlaybookName(name string) (PlaybookName, error) {
	switch PlaybookName(cleanName(name)) {
	case PlaybookTrend:
		return PlaybookTrend, nil
	case PlaybookDrive:
		return PlaybookDrive, nil
	case PlaybookLeadLag:
		return PlaybookLeadLag, nil
	case PlaybookScarcity:
		return PlaybookScarcity, nil
	case PlaybookPump:
		return PlaybookPump, nil
	default:
		return "", fmt.Errorf("unknown playbook %q", name)
	}
}

func parseRegime(name string) (Regime, error) {
	switch cleanName(name) {
	case "", "none":
		return RegimeNone, nil
	case "dead":
		return RegimeDead, nil
	case "choppy":
		return RegimeChoppy, nil
	case "trending":
		return RegimeTrending, nil
	case "bullish":
		return RegimeBullish, nil
	case "bearish":
		return RegimeBearish, nil
	default:
		return RegimeNone, fmt.Errorf("unknown regime %q", name)
	}
}

func parsePolicy(name string) (EntryPolicy, error) {
	switch cleanName(name) {
	case "", "standard":
		return EntryPolicyStandard, nil
	case "drive":
		return EntryPolicyDrive, nil
	case "pump":
		return EntryPolicyPump, nil
	default:
		return EntryPolicyStandard, fmt.Errorf("unknown policy %q", name)
	}
}

func parseAction(name string) (ActionType, error) {
	switch cleanName(name) {
	case "", "none":
		return ActionNone, nil
	case "enter":
		return ActionEnter, nil
	case "deny":
		return ActionDeny, nil
	case "wait":
		return ActionWait, nil
	case "stop_loss":
		return ActionStopLoss, nil
	case "take_profit":
		return ActionTakeProfit, nil
	case "short":
		return ActionShort, nil
	default:
		return ActionNone, fmt.Errorf("unknown action %q", name)
	}
}
