package tests

import "github.com/theapemachine/symm/types"

/*
Last returns the newest thesis from Play, or nil when no tick completed.
*/
func Last(theses []*types.Thesis) *types.Thesis {
	if len(theses) == 0 {
		return nil
	}

	return theses[len(theses)-1]
}

/*
PeakMetric returns the greatest raw value for symbol/metric across theses.
*/
func PeakMetric(
	theses []*types.Thesis,
	symbol string,
	metric types.MetricType,
) (float64, bool) {
	peak := 0.0
	found := false
	initialized := false

	for _, thesis := range theses {
		if thesis == nil {
			continue
		}

		for _, measurement := range thesis.Measurements {
			if measurement.Symbol != symbol || measurement.Metric != metric {
				continue
			}

			found = true

			if !initialized {
				peak = measurement.Raw
				initialized = true
				continue
			}

			if measurement.Raw > peak {
				peak = measurement.Raw
			}
		}
	}

	return peak, found
}

/*
PeakSourceMetric returns the greatest raw value for source/symbol/metric across
theses so shared metric names from different signals do not collide.
*/
func PeakSourceMetric(
	theses []*types.Thesis,
	source types.SourceType,
	symbol string,
	metric types.MetricType,
) (float64, bool) {
	return PeakSourceMetricTail(theses, source, symbol, metric, 0)
}

/*
PeakSourceMetricTail is PeakSourceMetric over the trailing fraction of theses
so shared condition prefixes cannot mask the stressed trajectory.
*/
func PeakSourceMetricTail(
	theses []*types.Thesis,
	source types.SourceType,
	symbol string,
	metric types.MetricType,
	skipFraction float64,
) (float64, bool) {
	if skipFraction < 0 {
		skipFraction = 0
	}

	if skipFraction > 0.9 {
		skipFraction = 0.9
	}

	start := int(float64(len(theses)) * skipFraction)
	peak := 0.0
	found := false
	initialized := false

	for _, thesis := range theses[start:] {
		if thesis == nil {
			continue
		}

		for _, measurement := range thesis.Measurements {
			if measurement.Source != source ||
				measurement.Symbol != symbol ||
				measurement.Metric != metric {
				continue
			}

			found = true

			if !initialized {
				peak = measurement.Raw
				initialized = true
				continue
			}

			if measurement.Raw > peak {
				peak = measurement.Raw
			}
		}
	}

	return peak, found
}
