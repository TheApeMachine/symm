package utils

import "github.com/theapemachine/symm/types"

/*
LatestMeasurements reduces the newest complete measurement epoch to values
grouped by metric and symbol so callers can inspect a signal's current state.
*/
func LatestMeasurements(
	measurements []*types.Measurement,
	source types.SourceType,
	metrics []types.MetricType,
) map[types.MetricType]map[string]float64 {
	sourced := make([]*types.Measurement, 0, len(measurements))

	for _, measurement := range measurements {
		if measurement.Source == source {
			sourced = append(sourced, measurement)
		}
	}

	return measurementValues(types.FilterLatest(sourced), source, metrics, false)
}

/*
PeakMeasurements reduces a measurement history to each symbol's largest value
for every requested metric so callers can inspect a transition's strongest evidence.
*/
func PeakMeasurements(
	measurements []*types.Measurement,
	source types.SourceType,
	metrics []types.MetricType,
) map[types.MetricType]map[string]float64 {
	return measurementValues(measurements, source, metrics, true)
}

/*
measurementValues groups requested measurements while selecting either the
latest supplied value or the largest value observed for each metric and symbol.
*/
func measurementValues(
	measurements []*types.Measurement,
	source types.SourceType,
	metrics []types.MetricType,
	peak bool,
) map[types.MetricType]map[string]float64 {
	values := make(map[types.MetricType]map[string]float64, len(metrics))

	for _, metric := range metrics {
		values[metric] = map[string]float64{}
	}

	for _, measurement := range measurements {
		metricValues, requested := values[measurement.Metric]

		if measurement.Source != source || !requested {
			continue
		}

		value, found := metricValues[measurement.Symbol]

		if peak && found && value >= measurement.Raw {
			continue
		}

		metricValues[measurement.Symbol] = measurement.Raw
	}

	return values
}
