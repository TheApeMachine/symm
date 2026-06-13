package logic

import "github.com/theapemachine/symm/config"

func applyConfigThresholds(tree *Tree, thresholdConfig config.ThresholdConfig) {
	for _, branch := range tree.Branches {
		applyBranchThresholds(
			branch,
			thresholdConfig.EntryConfidenceBaseline,
			thresholdConfig.ExitConfidenceBaseline,
			thresholdConfig.EntrySurpriseBaseline,
		)
	}
}

func applyBranchThresholds(
	branch *Branch, confidenceBaseline float64, exitConfidenceBaseline float64, surpriseBaseline float64,
) {
	if branch.ConditionGroup != nil {
		applyGroupThresholds(branch.ConditionGroup, confidenceBaseline, exitConfidenceBaseline, surpriseBaseline)
	}

	for _, child := range branch.Branches {
		applyBranchThresholds(child, confidenceBaseline, exitConfidenceBaseline, surpriseBaseline)
	}
}

func applyGroupThresholds(
	group *ConditionGroup, confidenceBaseline float64, exitConfidenceBaseline float64, surpriseBaseline float64,
) {
	for conditionIndex := range group.Conditions {
		applyConditionThresholds(
			&group.Conditions[conditionIndex],
			confidenceBaseline,
			exitConfidenceBaseline,
			surpriseBaseline,
		)
	}
}

func applyConditionThresholds(
	condition *Condition,
	confidenceBaseline float64,
	exitConfidenceBaseline float64,
	surpriseBaseline float64,
) {
	if !condition.Type.isComparison() {
		return
	}

	rightSubject := &condition.Right.Subject

	if rightSubject.confidenceUsesBaseline {
		rightSubject.Confidence = confidenceBaseline
	}

	if rightSubject.confidenceUsesExitBaseline {
		rightSubject.Confidence = exitConfidenceBaseline
	}

	if rightSubject.surpriseUsesBaseline {
		rightSubject.Surprise = surpriseBaseline
	}
}

func (conditionType ConditionType) isComparison() bool {
	switch conditionType {
	case ConditionIsEqual, ConditionIsNotEqual,
		ConditionIsGreaterThan, ConditionIsLessThan,
		ConditionIsGreaterThanOrEqual, ConditionIsLessThanOrEqual,
		ConditionIsWithin, ConditionIsNotWithin:
		return true
	default:
		return false
	}
}
