package optimizer

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

func (profile *Profile) categoryCount(category perspectives.CategoryType) int {
	profile.PrepareCache()

	return profile.categoryCounts[category]
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
