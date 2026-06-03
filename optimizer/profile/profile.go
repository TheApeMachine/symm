package profile

import (
	"context"
	"math"
	"sort"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
Profile holds replay measurements used to derive branch thresholds.
*/
type Profile struct {
	ctx            context.Context
	rows           []perspectives.Measurement
	sortedValues   map[string][]float64
	categoryCounts map[perspectives.CategoryType]int
	categories     []perspectives.CategoryType
	prepared       bool
}

func NewProfile(ctx context.Context) *Profile {
	return &Profile{ctx: ctx}
}

func (profile *Profile) Add(measurement perspectives.Measurement) {
	profile.rows = append(profile.rows, measurement)
	profile.prepared = false
}

func (profile *Profile) Len() int {
	return len(profile.rows)
}

func (profile *Profile) Rows() []perspectives.Measurement {
	rows := make([]perspectives.Measurement, len(profile.rows))
	copy(rows, profile.rows)

	return rows
}

/*
PrepareCache pre-sorts category metrics so quantiles and gate counts are O(log N).
Call once after loading the full measurement tape.
*/
func (profile *Profile) PrepareCache() {
	if profile.prepared {
		return
	}

	grouped := make(map[string][]float64)
	categoryCounts := make(map[perspectives.CategoryType]int)
	categorySeen := make(map[perspectives.CategoryType]struct{})
	categories := make([]perspectives.CategoryType, 0)

	for _, row := range profile.rows {
		if row.Category == perspectives.CategoryTypeNone {
			continue
		}

		categoryCounts[row.Category]++

		if _, seen := categorySeen[row.Category]; !seen {
			categorySeen[row.Category] = struct{}{}
			categories = append(categories, row.Category)
		}

		grouped[profileValueKey(row.Category, perspectives.UnitSNR)] =
			append(grouped[profileValueKey(row.Category, perspectives.UnitSNR)], row.SNR)
		grouped[profileValueKey(row.Category, perspectives.UnitConfidence)] =
			append(
				grouped[profileValueKey(row.Category, perspectives.UnitConfidence)],
				row.Confidence,
			)
	}

	sortedValues := make(map[string][]float64, len(grouped))

	for key, values := range grouped {
		sort.Float64s(values)
		sortedValues[key] = values
	}

	profile.sortedValues = sortedValues
	profile.categoryCounts = categoryCounts
	profile.categories = categories
	profile.prepared = true
}

func (profile *Profile) Categories() []perspectives.CategoryType {
	profile.PrepareCache()

	return profile.categories
}

func (profile *Profile) Quantile(
	category perspectives.CategoryType,
	unit perspectives.UnitType,
	percentile float64,
) float64 {
	profile.PrepareCache()

	values := profile.sortedValues[profileValueKey(category, unit)]

	if len(values) == 0 {
		return 0
	}

	index := int(math.Round(percentile * float64(len(values)-1)))

	if index < 0 {
		index = 0
	}

	if index >= len(values) {
		index = len(values) - 1
	}

	return values[index]
}

/*
JitterScale returns the IQR of a category/unit distribution for threshold perturbation.
When the IQR is zero, the absolute threshold magnitude is used instead.
*/
func (profile *Profile) JitterScale(
	category perspectives.CategoryType,
	unit perspectives.UnitType,
	value float64,
) float64 {
	profile.PrepareCache()

	q1 := profile.Quantile(category, unit, 0.25)
	q3 := profile.Quantile(category, unit, 0.75)
	iqr := q3 - q1

	if iqr > 0 {
		return iqr
	}

	return math.Max(math.Abs(value), 1e-9)
}

func (profile *Profile) Values(
	category perspectives.CategoryType,
	unit perspectives.UnitType,
	limit int,
) []float64 {
	profile.PrepareCache()

	values := uniqueSortedValues(profile.sortedValues[profileValueKey(category, unit)])

	if limit <= 0 || len(values) <= limit {
		return values
	}

	return sampleValues(values, limit)
}

/*
AdaptiveValues returns representative in-sample thresholds for stochastic search.

Unlike Values, which treats limit <= 0 as "all unique values" for exhaustive
beam scans, AdaptiveValues derives a compact threshold set from the observed
distribution when no explicit limit is supplied. This keeps MCTS moves tied to
current market conditions without hard-coding a small quantile grid.
*/
func (profile *Profile) AdaptiveValues(
	category perspectives.CategoryType,
	unit perspectives.UnitType,
	limit int,
) []float64 {
	profile.PrepareCache()

	values := uniqueSortedValues(profile.sortedValues[profileValueKey(category, unit)])

	if len(values) == 0 {
		return nil
	}

	if limit <= 0 {
		limit = adaptiveThresholdLimit(len(values))
	}

	if len(values) <= limit {
		return values
	}

	return sampleValues(values, limit)
}

func profileValueKey(
	category perspectives.CategoryType,
	unit perspectives.UnitType,
) string {
	switch unit {
	case perspectives.UnitSNR:
		return string(category) + ":snr"
	case perspectives.UnitConfidence:
		return string(category) + ":conf"
	default:
		return string(category) + ":unknown"
	}
}

func uniqueSortedValues(values []float64) []float64 {
	if len(values) == 0 {
		return values
	}

	unique := make([]float64, 0, len(values))
	lastValue := values[0]
	unique = append(unique, lastValue)

	for index := 1; index < len(values); index++ {
		if values[index] == lastValue {
			continue
		}

		lastValue = values[index]
		unique = append(unique, lastValue)
	}

	return unique
}

func sampleValues(values []float64, limit int) []float64 {
	if limit <= 1 {
		return []float64{values[0]}
	}

	sampled := make([]float64, 0, limit)
	lastIndex := len(values) - 1

	for sampleIndex := range limit {
		index := int(math.Round(
			float64(sampleIndex) * float64(lastIndex) / float64(limit-1),
		))
		value := values[index]

		if len(sampled) > 0 && sampled[len(sampled)-1] == value {
			continue
		}

		sampled = append(sampled, value)
	}

	return sampled
}

func adaptiveThresholdLimit(valueCount int) int {
	if valueCount <= 0 {
		return 0
	}

	if valueCount <= 3 {
		return valueCount
	}

	// Square-root growth gives richer thresholds in volatile tapes while avoiding
	// an exploding MCTS branching factor on long captures.
	limit := int(math.Round(math.Sqrt(float64(valueCount))))

	if limit < 3 {
		return 3
	}

	if limit > 16 {
		return 16
	}

	return limit
}
