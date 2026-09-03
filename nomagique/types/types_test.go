package types

import (
	"testing"
)

func TestMeasurementRetainsBoundaryMetadata(t *testing.T) {
	measurement := NewMeasurement("1", "hawkes", 0, 0)
	measurement.Put("event_count", NewMetric("event_count", 7.0, Descriptor{
		Unit:      UnitCount,
		Timescale: TimescalePerSecond,
	}))
	metric := measurement.Metric("event_count")

	if metric == nil || metric.Raw != 7 || metric.Unit != UnitCount {
		t.Fatal("measurement did not retain its metric")
	}

	if measurement.Metric("missing") != nil || measurement.Error() == "" {
		t.Fatal("missing metric should be reported")
	}
}
