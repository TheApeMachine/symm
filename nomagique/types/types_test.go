package types

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/types"
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

func TestFrameMigrationAliases(t *testing.T) {
	symbol := MustIntern("types_test/value")
	frame := types.Frame{}
	frame.Put(symbol, 3)

	if frame.MustGet(symbol) != 3 {
		t.Fatal("Frame alias did not retain the value")
	}
}
