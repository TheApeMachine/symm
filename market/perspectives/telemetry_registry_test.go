package perspectives

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTelemetryRegistryObserveMeasurement(t *testing.T) {
	ResetTelemetryRegistryForTest()
	t.Cleanup(ResetTelemetryRegistryForTest)

	Convey("Given a measurement from a new source", t, func() {
		registry := NewTelemetryRegistry()

		registry.ObserveMeasurement(Measurement{
			Source: SourcePumpDump,
		})

		Convey("It should expose the source in manifest order", func() {
			So(registry.Names(), ShouldResemble, []string{"pumpdump"})
			So(registry.Label("pumpdump"), ShouldEqual, "Pump")
		})
	})
}

func BenchmarkTelemetryRegistryRegister(b *testing.B) {
	registry := NewTelemetryRegistry()

	for b.Loop() {
		registry.Register("bench_source", "Bench")
	}
}
