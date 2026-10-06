package strategy

import (
	"github.com/theapemachine/symm/nomagique/data"
)

/*
readMetric returns the first metric written under key on measurement.

Absence is not an error: a measurement that never wrote key yields
(nil, nil), and the caller decides whether that key is required. A read
failure (e.g. a measurement that is not finalized) yields its error.
Callers that require the metric must turn a nil metric into their own
hard failure; this never invents a value.
*/
func readMetric(measurement *data.Measurement, key string) (*data.Metric, error) {
	if measurement == nil {
		return nil, nil
	}

	entry := data.Pull(measurement.Read(key))

	if entry == nil {
		return nil, nil
	}

	if entry.Err != nil {
		return nil, entry.Err
	}

	if entry.Metric == nil || entry.Metric.Label != key {
		return nil, nil
	}

	return entry.Metric, nil
}
