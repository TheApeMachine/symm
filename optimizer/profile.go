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
