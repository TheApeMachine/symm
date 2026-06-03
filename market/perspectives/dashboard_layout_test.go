package perspectives

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSplitDashboardGaugeSources(t *testing.T) {
	Convey("Given thirteen registered telemetry sources", t, func() {
		registered := []string{
			"toxicity",
			"hawkes",
			"fluid",
			"correlation",
			"pumpdump",
			"causal",
			"depthflow",
			"leadlag",
			"liquidity",
			"sentiment",
			"exhaustion",
			"prediction",
			"cvd",
		}

		grid, strip := SplitDashboardGaugeSources(registered)

		Convey("It should keep eight primary gauges in the top grid", func() {
			So(len(grid), ShouldEqual, DashboardGaugeGridCapacity)
			So(grid, ShouldResemble, []string{
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

		Convey("It should move overflow gauges to the side strip", func() {
			So(strip, ShouldResemble, []string{
				"toxicity",
				"correlation",
				"exhaustion",
				"prediction",
				"cvd",
			})
		})
	})
}

func BenchmarkSplitDashboardGaugeSources(b *testing.B) {
	ResetTelemetryRegistryForTest()
	BootstrapTelemetryManifest()
	registered := DefaultTelemetryRegistry().Names()

	for b.Loop() {
		_, _ = SplitDashboardGaugeSources(registered)
	}
}
