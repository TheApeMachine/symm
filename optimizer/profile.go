package optimizer

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
	ctx  context.Context
	rows []perspectives.Measurement
}

func (profile *Profile) Add(measurement perspectives.Measurement) {
	profile.rows = append(profile.rows, measurement)
}

func (profile *Profile) Len() int {
	return len(profile.rows)
}

func (profile *Profile) Rows() []perspectives.Measurement {
	rows := make([]perspectives.Measurement, len(profile.rows))
	copy(rows, profile.rows)

	return rows
}

func (profile *Profile) Categories() []perspectives.CategoryType {
	seen := make(map[perspectives.CategoryType]struct{})
	order := make([]perspectives.CategoryType, 0)

	for _, row := range profile.rows {
		if row.Category == perspectives.CategoryTypeNone {
			continue
		}

		if _, ok := seen[row.Category]; ok {
			continue
		}

		seen[row.Category] = struct{}{}
		order = append(order, row.Category)
	}

	return order
}

func (profile *Profile) Quantile(
	category perspectives.CategoryType,
	unit perspectives.UnitType,
	percentile float64,
) float64 {
	values := make([]float64, 0)

	for _, row := range profile.rows {
		if row.Category != category {
			continue
		}

		switch unit {
		case perspectives.UnitSNR:
			values = append(values, row.SNR)
		case perspectives.UnitConfidence:
			values = append(values, row.Confidence)
		default:
			continue
		}
	}

	if len(values) == 0 {
		return 0
	}

	sort.Float64s(values)

	index := int(math.Round(percentile * float64(len(values)-1)))

	if index < 0 {
		index = 0
	}

	if index >= len(values) {
		index = len(values) - 1
	}

	return values[index]
}

func (profile *Profile) Values(
	category perspectives.CategoryType,
	unit perspectives.UnitType,
	limit int,
) []float64 {
	seen := make(map[float64]struct{})
	values := make([]float64, 0)

	for _, row := range profile.rows {
		if row.Category != category {
			continue
		}

		value, ok := profile.value(row, unit)

		if !ok {
			continue
		}

		if _, ok := seen[value]; ok {
			continue
		}

		seen[value] = struct{}{}
		values = append(values, value)
	}

	sort.Float64s(values)

	if limit <= 0 || len(values) <= limit {
		return values
	}

	return sampleValues(values, limit)
}

func (profile *Profile) value(
	row perspectives.Measurement,
	unit perspectives.UnitType,
) (float64, bool) {
	switch unit {
	case perspectives.UnitSNR:
		return row.SNR, true
	case perspectives.UnitConfidence:
		return row.Confidence, true
	default:
		return 0, false
	}
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
