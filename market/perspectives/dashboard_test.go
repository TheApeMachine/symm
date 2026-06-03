package perspectives

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTelemetryRegistry(t *testing.T) {
	ResetTelemetryRegistryForTest()
	t.Cleanup(ResetTelemetryRegistryForTest)

	Convey("Given bootstrap telemetry manifest", t, func() {
		ResetTelemetryRegistryForTest()
		BootstrapTelemetryManifest()

		Convey("It should seed known dashboard sources", func() {
			So(DefaultTelemetryRegistry().Names(), ShouldContain, "hawkes")
			So(DefaultTelemetryRegistry().Names(), ShouldContain, "fluid")
		})
	})

	Convey("Given an empty telemetry registry", t, func() {
		registry := NewTelemetryRegistry()

		Convey("It should register sources from measurements", func() {
			registry.ObserveMeasurement(Measurement{
				Source: SourceHawkes,
			})

			So(registry.Register("hawkes", "Hawkes"), ShouldBeFalse)
			So(registry.Names(), ShouldResemble, []string{"hawkes"})
			So(registry.Label("hawkes"), ShouldEqual, "Hawkes")
		})

		Convey("It should title-case unknown source labels", func() {
			registry.Register("alpha_model", "")

			So(registry.Label("alpha_model"), ShouldEqual, "Alpha_model")
		})

		Convey("It should notify subscribers when the manifest grows", func() {
			notified := 0

			registry.Subscribe(func() {
				notified++
			})
			registry.Register("fluid", "Fluid")

			So(notified, ShouldEqual, 1)
			So(registry.Version(), ShouldEqual, 1)
		})
	})
}

func TestDashboardGaugeNames(t *testing.T) {
	ResetTelemetryRegistryForTest()
	t.Cleanup(ResetTelemetryRegistryForTest)

	Convey("Given registered telemetry sources", t, func() {
		registry := DefaultTelemetryRegistry()

		for _, name := range []string{
			"hawkes",
			"fluid",
			"pumpdump",
			"causal",
			"depthflow",
			"leadlag",
			"liquidity",
			"sentiment",
		} {
			registry.Register(name, SourceDisplayLabel(name))
		}

		names := DashboardGaugeNames()

		Convey("It should expose the registered gauge source names", func() {
			So(names, ShouldResemble, []string{
				"hawkes",
				"fluid",
				"pumpdump",
				"causal",
				"depthflow",
				"leadlag",
				"liquidity",
				"sentiment",
			})
		})

		Convey("It should label each dashboard source", func() {
			So(DashboardGaugeLabel("fluid"), ShouldEqual, "Fluid")
			So(DashboardGaugeLabel("liquidity"), ShouldEqual, "Liquidity")
			So(DashboardGaugeLabel("unknown"), ShouldEqual, "Unknown")
		})
	})
}

func BenchmarkDashboardGaugeNames(b *testing.B) {
	ResetTelemetryRegistryForTest()

	for _, name := range []string{"hawkes", "fluid", "pumpdump"} {
		DefaultTelemetryRegistry().Register(name, SourceDisplayLabel(name))
	}

	for b.Loop() {
		_ = DashboardGaugeNames()
	}
}
