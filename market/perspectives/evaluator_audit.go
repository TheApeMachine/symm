package perspectives

/*
ActionAudited returns the deepest reachable branch action and fills audit.
*/
func (evaluator *BranchEvaluator) ActionAudited(
	branches BranchList, audit *WalkAudit,
) *ActionType {
	evaluator.audit = audit

	decision := evaluator.walk(branches, 0, nil)

	if audit != nil {
		audit.VerdictDepth = decision.depth

		if decision.found {
			audit.Verdict = &decision.actionType
			audit.markSelectedPath(decision.path)
		}
	}

	evaluator.audit = nil

	if !decision.found {
		return nil
	}

	return &decision.actionType
}

func (evaluator *BranchEvaluator) checkBranch(branch Branch) branchCheck {
	if check := evaluator.checkRegime(branch); !check.pass {
		return check
	}

	if check := evaluator.checkObservation(branch); !check.pass {
		return check
	}

	if check := evaluator.checkAction(branch); !check.pass {
		return check
	}

	return evaluator.checkCategory(branch)
}

func (evaluator *BranchEvaluator) checkRegime(branch Branch) branchCheck {
	if branch.Regime == RegimeNone {
		return branchCheck{pass: true}
	}

	pass := evaluator.context.Regime == branch.Regime

	if pass {
		return branchCheck{pass: true}
	}

	return branchCheck{
		pass:   false,
		reason: "regime",
		compared: &ComparedValue{
			Field: "regime",
			Left:  float64(evaluator.context.Regime),
			Op:    ConditionIsEqual,
			Right: float64(branch.Regime),
			Pass:  false,
		},
	}
}

func (evaluator *BranchEvaluator) checkObservation(branch Branch) branchCheck {
	if branch.Observation == ObservationNone {
		return branchCheck{pass: true}
	}

	value, present := evaluator.context.Observations[branch.Observation]

	if branch.Unit != UnitNone {
		if present {
			return branchCheck{pass: true}
		}

		return branchCheck{pass: false, reason: "observation_missing"}
	}

	switch branch.Condition {
	case ConditionIsFalse:
		pass := !present || value == 0

		return branchCheck{
			pass:   pass,
			reason: observationFailReason(pass),
			compared: &ComparedValue{
				Field: branch.Observation.String(),
				Left:  value,
				Op:    ConditionIsFalse,
				Pass:  pass,
			},
		}
	case ConditionIsTrue:
		pass := present && value != 0

		return branchCheck{
			pass:   pass,
			reason: observationFailReason(pass),
			compared: &ComparedValue{
				Field: branch.Observation.String(),
				Left:  value,
				Op:    ConditionIsTrue,
				Pass:  pass,
			},
		}
	default:
		if present {
			return branchCheck{pass: true}
		}

		return branchCheck{pass: false, reason: "observation_missing"}
	}
}

func observationFailReason(pass bool) string {
	if pass {
		return ""
	}

	return "observation"
}

func (evaluator *BranchEvaluator) checkAction(branch Branch) branchCheck {
	if branch.Action.Type == ActionNone {
		return branchCheck{pass: true}
	}

	if branch.Observation == ObservationNotHolding {
		if evaluator.isEntryAction(branch.Action.Type) {
			return branchCheck{pass: true}
		}

		return branchCheck{pass: false, reason: "entry_action_incompatible"}
	}

	if branch.Observation == ObservationHolding {
		if evaluator.isExitAction(branch.Action.Type) {
			return branchCheck{pass: true}
		}

		return branchCheck{pass: false, reason: "exit_action_incompatible"}
	}

	evaluator.fail(
		"action %d requires holding or not_holding observation",
		branch.Action.Type,
	)

	return branchCheck{pass: false, reason: "action_observation_required"}
}

func (evaluator *BranchEvaluator) checkCategory(branch Branch) branchCheck {
	if branch.Category == CategoryTypeNone {
		return evaluator.checkNumeric(branch, Measurement{})
	}

	var lastCheck branchCheck

	for _, measurement := range evaluator.context.Measurements {
		if measurement.Category != branch.Category {
			continue
		}

		check := evaluator.checkNumeric(branch, measurement)

		if check.pass {
			return check
		}

		lastCheck = check
	}

	if lastCheck.reason != "" {
		return lastCheck
	}

	return branchCheck{pass: false, reason: "category"}
}

func (evaluator *BranchEvaluator) checkNumeric(
	branch Branch, measurement Measurement,
) branchCheck {
	if branch.Unit == UnitNone && !branch.hasNumericCondition() {
		return branchCheck{pass: true}
	}

	if branch.Unit == UnitNone {
		evaluator.fail(
			"numeric branch condition %d has no unit",
			branch.Condition,
		)

		return branchCheck{pass: false, reason: "numeric_unit_missing"}
	}

	if branch.Condition == ConditionNone {
		evaluator.fail(
			"branch unit %d has no condition",
			branch.Unit,
		)

		return branchCheck{pass: false, reason: "numeric_condition_missing"}
	}

	value, ok := evaluator.value(measurement, branch)

	if !ok {
		return branchCheck{pass: false, reason: "metric_missing"}
	}

	pass := evaluator.compare(value, branch.Value, branch.Condition)
	field := comparedField(branch)

	return branchCheck{
		pass:   pass,
		reason: numericFailReason(pass),
		compared: &ComparedValue{
			Field: field,
			Left:  value,
			Op:    branch.Condition,
			Right: branch.Value,
			Pass:  pass,
		},
	}
}

func comparedField(branch Branch) string {
	if trimmed := branch.Metric; trimmed != "" {
		return trimmed
	}

	switch branch.Unit {
	case UnitSNR:
		return "snr"
	case UnitConfidence:
		return "confidence"
	default:
		return branch.Unit.String()
	}
}

func numericFailReason(pass bool) string {
	if pass {
		return ""
	}

	return "numeric"
}
