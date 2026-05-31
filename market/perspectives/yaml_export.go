package perspectives

/*
BuiltinDocument is the version-controlled YAML equivalent of the Go playbook
constructors (NewTrendPerspective, NewDrivePerspective, etc.).
*/
func BuiltinDocument() Document {
	return Document{
		Version: 1,
		Playbooks: []PlaybookSpec{
			exportPlaybook(NewTrendPerspective()),
			exportPlaybook(NewDrivePerspective()),
			exportPlaybook(NewLeadLagPerspective()),
			exportPlaybook(NewScarcityPerspective()),
			exportPlaybook(NewPumpPerspective()),
		},
	}
}

func exportPlaybook(strat *strategy) PlaybookSpec {
	spec := PlaybookSpec{
		Name:   string(strat.name),
		Regime: regimeLabel(strat.regime),
		Policy: policyLabel(strat.policy),
		Entry:  exportBranches(strat.entry.Branches),
	}

	if strat.deny != nil {
		spec.Deny = exportBranches(strat.deny.Branches)
	}

	if len(strat.exit.Branches) > 0 {
		spec.Exit = exportExitBranches(strat.exit.Branches[0])
	}

	return spec
}

func exportExitBranches(exit Branch) []BranchSpec {
	if exit.Observation == ObservationHolding {
		return exportBranches(exit.Branches)
	}

	return exportBranches([]Branch{exit})
}

func exportBranches(branches []Branch) []BranchSpec {
	if len(branches) == 0 {
		return nil
	}

	if len(branches) == 1 {
		return []BranchSpec{exportBranchSpec(branches[0])}
	}

	if branchesAreAlternatives(branches) {
		alternatives := make([]BranchSpec, 0, len(branches))

		for _, branch := range branches {
			alternatives = append(alternatives, exportBranchSpec(branch))
		}

		return []BranchSpec{{Any: alternatives}}
	}

	specs := make([]BranchSpec, 0, len(branches))

	for _, branch := range branches {
		specs = append(specs, exportBranchSpec(branch))
	}

	return specs
}

func exportBranchSpec(branch Branch) BranchSpec {
	spec := BranchSpec{
		Condition: conditionLabel(branch.Condition),
	}

	if branch.Category != CategoryTypeNone {
		spec.Category = branch.Category.String()
	}

	if branch.Observation != ObservationNone {
		spec.Observation = observationLabel(branch.Observation)
	}

	if branch.Metric != "" {
		spec.Metric = branch.Metric
	}

	if branch.Action != ActionNone {
		spec.Action = ActionLabel(branch.Action)
	}

	if branch.ValueSet {
		value := branch.Value
		spec.Value = &value
	}

	if len(branch.Branches) > 0 {
		spec.Branches = exportBranches(branch.Branches)
	}

	return spec
}

func branchesAreAlternatives(branches []Branch) bool {
	for _, branch := range branches {
		if branch.Observation != ObservationNone && branch.Observation != ObservationHolding {
			return false
		}

		if branch.Metric != "" {
			return false
		}
	}

	return true
}

func regimeLabel(regime Regime) string {
	switch regime {
	case RegimeDead:
		return "dead"
	case RegimeChoppy:
		return "choppy"
	case RegimeTrending:
		return "trending"
	case RegimeBullish:
		return "bullish"
	case RegimeBearish:
		return "bearish"
	default:
		return "none"
	}
}

func policyLabel(policy EntryPolicy) string {
	switch policy {
	case EntryPolicyDrive:
		return "drive"
	case EntryPolicyPump:
		return "pump"
	default:
		return "standard"
	}
}

func conditionLabel(condition ConditionType) string {
	switch condition {
	case ConditionIsGreaterThanOrEqual:
		return ">="
	case ConditionIsLessThan:
		return "<"
	case ConditionIsLessThanOrEqual:
		return "<="
	case ConditionIsEqual:
		return "="
	case ConditionIsNotEqual:
		return "!="
	default:
		return ">"
	}
}

func observationLabel(observation ObservationType) string {
	switch observation {
	case ObservationHasStarted:
		return "has_started"
	case ObservationHasContinued:
		return "has_continued"
	case ObservationHasEnded:
		return "has_ended"
	case ObservationHasDoneBefore:
		return "has_done_before"
	case ObservationHolding:
		return "holding"
	case ObservationNotHolding:
		return "not_holding"
	default:
		return "none"
	}
}
