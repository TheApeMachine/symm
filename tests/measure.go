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

		row.EachMetric(func(metric types.MetricType, side types.MeasurementSide, sample types.MetricSample) bool {
			if side != types.SideNone {
				return true
			}

			if _, ok := wanted[metric]; !ok {
				return true
			}

			bySymbol, ok := out[metric]

			if !ok {
				bySymbol = map[string]float64{}
				out[metric] = bySymbol
			}

			current, seen := bySymbol[row.Symbol]

			if !seen || sample.Raw > current {
				bySymbol[row.Symbol] = sample.Raw
			}

			return true
		})
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

		nano := row.At.UnixNano()

		row.EachMetric(func(metric types.MetricType, side types.MeasurementSide, sample types.MetricSample) bool {
			if side != types.SideNone {
				return true
			}

			if _, ok := wanted[metric]; !ok {
				return true
			}

			bySymbol, ok := latest[metric]

			if !ok {
				bySymbol = map[string]stamp{}
				latest[metric] = bySymbol
			}

			current, seen := bySymbol[row.Symbol]

			if !seen || nano >= current.at {
				bySymbol[row.Symbol] = stamp{at: nano, raw: sample.Raw}
			}

			return true
		})
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

		row.EachMetric(func(metric types.MetricType, side types.MeasurementSide, sample types.MetricSample) bool {
			if side != types.SideNone {
				return true
			}

			if _, ok := wanted[metric]; !ok {
				return true
			}

			bySymbol, ok := out[metric]

			if !ok {
				bySymbol = map[string]float64{}
				out[metric] = bySymbol
			}

			current, seen := bySymbol[row.Symbol]

			if !seen || math.Abs(sample.Raw) > math.Abs(current) {
				bySymbol[row.Symbol] = sample.Raw
			}

			return true
		})
	}

	return out
}
