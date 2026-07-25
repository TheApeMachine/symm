package types

import "strings"

/*
MetricKey builds the Metrics map key for one metric, with an optional side
suffix so buy/sell (and kernel) samples are not clobbered. Matches the UI wire
convention used by AggregateMeasurements.
*/
func MetricKey(metric MetricType, side MeasurementSide) string {
	if side == SideNone || side == "" {
		return string(metric)
	}

	return string(metric) + ":" + string(side)
}

/*
ParseMetricKey splits a Metrics map key into metric and side.
*/
func ParseMetricKey(key string) (MetricType, MeasurementSide) {
	metric, side, ok := strings.Cut(key, ":")

	if !ok {
		return MetricType(key), SideNone
	}

	return MetricType(metric), MeasurementSide(side)
}

/*
Sample returns the Metrics entry for metric/side when present.
*/
func (measurement *Measurement) Sample(
	metric MetricType, side MeasurementSide,
) (MetricSample, bool) {
	if measurement == nil || len(measurement.Metrics) == 0 {
		return MetricSample{}, false
	}

	sample, ok := measurement.Metrics[MetricKey(metric, side)]

	return sample, ok
}

/*
PutMetric writes one sample into Metrics, allocating the map on first use.
*/
func (measurement *Measurement) PutMetric(
	metric MetricType, side MeasurementSide, sample MetricSample,
) {
	if measurement == nil {
		return
	}

	if measurement.Metrics == nil {
		measurement.Metrics = map[string]MetricSample{}
	}

	measurement.Metrics[MetricKey(metric, side)] = sample
}

/*
EachMetric ranges Metrics and invokes yield with parsed metric, side, and sample.
Yield returns false to stop iteration early.
*/
func (measurement *Measurement) EachMetric(
	yield func(metric MetricType, side MeasurementSide, sample MetricSample) bool,
) {
	if measurement == nil {
		return
	}

	for key, sample := range measurement.Metrics {
		metric, side := ParseMetricKey(key)

		if !yield(metric, side, sample) {
			return
		}
	}
}
