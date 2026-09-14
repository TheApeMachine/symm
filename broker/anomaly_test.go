package broker

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAnomalyMonitor(t *testing.T) {
	Convey("Given an AnomalyMonitor instance", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		monitor := NewAnomalyMonitor(ctx, 128)
		defer monitor.Close()

		Convey("When no anomalies have been recorded", func() {
			Convey("Then initial counts are zero and health is 1.0", func() {
				So(monitor.Count("BTC/USD"), ShouldEqual, 0)
				So(monitor.Total(), ShouldEqual, 0)
				So(monitor.Health("BTC/USD"), ShouldEqual, 1.0)
			})
		})

		Convey("When anomalies are recorded", func() {
			monitor.Record("BTC/USD", AnomalyCrossedBook)
			monitor.Record("BTC/USD", AnomalyInsufficientDepth)
			monitor.Record("ETH/USD", AnomalyIncompleteBook)

			time.Sleep(20 * time.Millisecond)

			Convey("Then counts and health reflect recorded events", func() {
				So(monitor.Count("BTC/USD"), ShouldEqual, 2)
				So(monitor.Count("ETH/USD"), ShouldEqual, 1)
				So(monitor.Total(), ShouldEqual, 3)

				So(monitor.Health("BTC/USD"), ShouldBeLessThan, 1.0)
				So(monitor.Health("BTC/USD"), ShouldEqual, 1.0/(1.0+2.0))
				So(monitor.Health("ETH/USD"), ShouldEqual, 1.0/(1.0+1.0))
				So(monitor.Health("SOL/USD"), ShouldEqual, 1.0)
			})
		})
	})
}
