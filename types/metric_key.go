package types

import (
	"sort"
	"strings"
	"sync"

	"github.com/theapemachine/errnie"
)

type metricSideKey struct {
	metric MetricType
	side   MeasurementSide
}

var metricKeysCache sync.Map

/*
MetricKey builds the Metrics map key for one metric, with an optional side
suffix so buy/sell samples do not clobber each other.
*/
func MetricKey(metric MetricType, side MeasurementSide) string {
	if side == SideNone || side == "" {
		return string(metric)
	}

	key := metricSideKey{metric: metric, side: side}
	if cached, ok := metricKeysCache.Load(key); ok {
		return cached.(string)
	}

	combined := string(metric) + ":" + string(side)
	actual, _ := metricKeysCache.LoadOrStore(key, combined)

	return actual.(string)
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
Sample returns the Metrics entry for metric and side when present.
*/
func (measurement *Measurement) Sample(
	metric MetricType,
	side MeasurementSide,
) MetricSample {
	if measurement == nil || len(measurement.Metrics) == 0 {
		return MetricSample{}
	}

	sample, ok := measurement.Metrics[MetricKey(metric, side)]

	if !ok {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"sample not found",
			nil,
		))
	}

	return sample
}

/*
PutMetric writes one sample into Metrics, allocating the map on first use.
*/
func (measurement *Measurement) PutMetric(
	metric MetricType,
	side MeasurementSide,
	sample MetricSample,
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
EachMetric ranges Metrics and invokes yield with parsed metric, side, and
sample. Yield returns false to stop iteration early.
*/
func (measurement *Measurement) EachMetric(
	yield func(metric MetricType, side MeasurementSide, sample MetricSample) bool,
) {
	if measurement == nil {
		return
	}

	keys := make([]string, 0, len(measurement.Metrics))

	for key := range measurement.Metrics {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, key := range keys {
		sample := measurement.Metrics[key]
		metric, side := ParseMetricKey(key)

		if !yield(metric, side, sample) {
			return
		}
	}
}
