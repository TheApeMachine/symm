package tests

import (
	"math"

	"github.com/theapemachine/symm/types"
)

/*
PeakMeasurements keeps the largest Raw per metric and symbol for source.
*/
func PeakMeasurements(
	rows []*types.Measurement,
	source types.SourceType,
	metrics []types.MetricType,
) map[types.MetricType]map[string]float64 {
	wanted := make(map[types.MetricType]struct{}, len(metrics))

	for _, metric := range metrics {
		wanted[metric] = struct{}{}
	}

	out := map[types.MetricType]map[string]float64{}

	for _, row := range rows {
		if row == nil || row.Source != source {
			continue
		}

		if _, ok := wanted[row.Metric]; !ok {
			continue
		}

		bySymbol, ok := out[row.Metric]

		if !ok {
			bySymbol = map[string]float64{}
			out[row.Metric] = bySymbol
		}

		current, seen := bySymbol[row.Symbol]

		if !seen || row.Raw > current {
			bySymbol[row.Symbol] = row.Raw
		}
	}

	return out
}

/*
LatestMeasurements keeps the last Raw by At per metric and symbol for source.
*/
func LatestMeasurements(
	rows []*types.Measurement,
	source types.SourceType,
	metrics []types.MetricType,
) map[types.MetricType]map[string]float64 {
	wanted := make(map[types.MetricType]struct{}, len(metrics))

	for _, metric := range metrics {
		wanted[metric] = struct{}{}
	}

	type stamp struct {
		at  int64
		raw float64
	}

	latest := map[types.MetricType]map[string]stamp{}

	for _, row := range rows {
		if row == nil || row.Source != source {
			continue
		}

		if _, ok := wanted[row.Metric]; !ok {
			continue
		}

		bySymbol, ok := latest[row.Metric]

		if !ok {
			bySymbol = map[string]stamp{}
			latest[row.Metric] = bySymbol
		}

		nano := row.At.UnixNano()
		current, seen := bySymbol[row.Symbol]

		if !seen || nano >= current.at {
			bySymbol[row.Symbol] = stamp{at: nano, raw: row.Raw}
		}
	}

	out := map[types.MetricType]map[string]float64{}

	for metric, bySymbol := range latest {
		values := map[string]float64{}

		for symbol, stamp := range bySymbol {
			values[symbol] = stamp.raw
		}

		out[metric] = values
	}

	return out
}

/*
PeakMagnitudeMeasurements keeps the largest |Raw| (preserving sign) per metric.
*/
func PeakMagnitudeMeasurements(
	rows []*types.Measurement,
	source types.SourceType,
	metrics []types.MetricType,
) map[types.MetricType]map[string]float64 {
	wanted := make(map[types.MetricType]struct{}, len(metrics))

	for _, metric := range metrics {
		wanted[metric] = struct{}{}
	}

	out := map[types.MetricType]map[string]float64{}

	for _, row := range rows {
		if row == nil || row.Source != source {
			continue
		}

		if _, ok := wanted[row.Metric]; !ok {
			continue
		}

		bySymbol, ok := out[row.Metric]

		if !ok {
			bySymbol = map[string]float64{}
			out[row.Metric] = bySymbol
		}

		current, seen := bySymbol[row.Symbol]

		if !seen || math.Abs(row.Raw) > math.Abs(current) {
			bySymbol[row.Symbol] = row.Raw
		}
	}

	return out
}
