package perspectives

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDashboardGaugeNames(t *testing.T) {
	Convey("Given dashboard gauge sources", t, func() {
		names := DashboardGaugeNames()

		Convey("It should expose the canonical gauge source names", func() {
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
			So(DashboardGaugeLabel("unknown"), ShouldEqual, "unknown")
		})
	})
}

func BenchmarkDashboardGaugeNames(b *testing.B) {
	for b.Loop() {
		_ = DashboardGaugeNames()
	}
}
