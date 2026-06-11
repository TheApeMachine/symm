package logic

import (
	"github.com/spf13/viper"
)

const (
	entryConfidenceLiteral = 0.55
	entrySurpriseLiteral   = 1.0
)

func applyConfigThresholds(tree *Tree) {
	confidenceBaseline := viper.GetFloat64("trading.entry.confidence_baseline")

	if confidenceBaseline <= 0 {
		confidenceBaseline = entryConfidenceLiteral
	}

	surpriseBaseline := viper.GetFloat64("trading.entry.surprise_baseline")

	if surpriseBaseline <= 0 {
		surpriseBaseline = entrySurpriseLiteral
	}

	for _, branch := range tree.Branches {
		applyBranchThresholds(branch, confidenceBaseline, surpriseBaseline)
	}
}

func applyBranchThresholds(
	branch *Branch, confidenceBaseline float64, surpriseBaseline float64,
) {
	if branch.ConditionGroup != nil {
		applyGroupThresholds(branch.ConditionGroup, confidenceBaseline, surpriseBaseline)
	}

	for _, child := range branch.Branches {
		applyBranchThresholds(child, confidenceBaseline, surpriseBaseline)
	}
}

func applyGroupThresholds(
	group *ConditionGroup, confidenceBaseline float64, surpriseBaseline float64,
) {
	for conditionIndex := range group.Conditions {
		applyConditionThresholds(
			&group.Conditions[conditionIndex],
			confidenceBaseline,
			surpriseBaseline,
		)
	}
}

func applyConditionThresholds(
	condition *Condition,
	confidenceBaseline float64,
	surpriseBaseline float64,
) {
	if !condition.Type.isComparison() {
		return
	}

	rightSubject := &condition.Right.Subject

	if rightSubject.Type == SubjectConfidence &&
		rightSubject.Confidence == entryConfidenceLiteral {
		rightSubject.Confidence = confidenceBaseline
	}

	if rightSubject.Type == SubjectSurprise &&
		rightSubject.Surprise == entrySurpriseLiteral {
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
