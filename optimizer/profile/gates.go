package profile

import (
	"sort"

	"github.com/theapemachine/symm/market/perspectives"
)

func (profile *Profile) GatePassCount(
	category perspectives.CategoryType,
	unit perspectives.UnitType,
	condition perspectives.ConditionType,
	threshold float64,
) int {
	profile.PrepareCache()

	values := profile.sortedValues[profileValueKey(category, unit)]

	return countPassingValues(values, threshold, condition)
}

func (profile *Profile) CategoryCount(category perspectives.CategoryType) int {
	profile.PrepareCache()

	return profile.categoryCounts[category]
}

/*
IsInformativeGate reports whether a threshold gate fires on a strict subset of
its category's readings — neither always (e.g. `snr >= 0`, vacuous because SNR is
clamped at 0) nor never. Only informative gates can discriminate, so the search
generators use this to skip vacuous moves that would otherwise pollute the tree
and waste budget. A gate over an unseen category is treated as non-informative.
*/
func (profile *Profile) IsInformativeGate(
	category perspectives.CategoryType,
	unit perspectives.UnitType,
	condition perspectives.ConditionType,
	threshold float64,
) bool {
	profile.PrepareCache()

	total := profile.categoryCounts[category]

	if total == 0 {
		return false
	}

	passes := profile.GatePassCount(category, unit, condition, threshold)

	return passes > 0 && passes < total
}

func countPassingValues(
	values []float64,
	threshold float64,
	condition perspectives.ConditionType,
) int {
	if len(values) == 0 {
		return 0
	}

	switch condition {
	case perspectives.ConditionIsGreaterThanOrEqual:
		return len(values) - sort.SearchFloat64s(values, threshold)
	case perspectives.ConditionIsGreaterThan:
		return len(values) - countLTEValues(values, threshold)
	case perspectives.ConditionIsLessThanOrEqual:
		return countLTEValues(values, threshold)
	case perspectives.ConditionIsLessThan:
		return sort.SearchFloat64s(values, threshold)
	default:
		return 0
	}
}

func countLTEValues(values []float64, threshold float64) int {
	return sort.Search(len(values), func(index int) bool {
		return values[index] > threshold
	})
}

func (profile *Profile) GateSelectivityScore(
	category perspectives.CategoryType,
	unit perspectives.UnitType,
	condition perspectives.ConditionType,
	threshold float64,
) float64 {
	profile.PrepareCache()

	categoryTotal := profile.categoryCounts[category]

	if categoryTotal == 0 || profile.Len() == 0 {
		return 0
	}

	passes := profile.GatePassCount(category, unit, condition, threshold)

	if passes == 0 {
		return 0
	}

	categoryWeight := float64(categoryTotal) / float64(profile.Len())
	passRate := float64(passes) / float64(categoryTotal)

	return categoryWeight * gateSelectivity(passRate)
}

func GateSelectivity(passRate float64) float64 {
	return gateSelectivity(passRate)
}

func gateSelectivity(passRate float64) float64 {
	if passRate <= 0 || passRate >= 1 {
		return 0
	}

	return 4 * passRate * (1 - passRate)
}
