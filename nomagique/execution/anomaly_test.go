package execution

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAnomalyMonitor(t *testing.T) {
	Convey("Given an AnomalyMonitor", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		monitor := NewAnomalyMonitor(ctx, 64)
		defer monitor.Close()

		Convey("Initial state has perfect health", func() {
			So(monitor.Health("TEST/ASSET"), ShouldEqual, 1.0)
			So(monitor.Count("TEST/ASSET"), ShouldEqual, 0)
			So(monitor.Total(), ShouldEqual, 0)
		})

		Convey("When recording anomalies", func() {
			monitor.Record("TEST/ASSET", AnomalyCrossedBook)
			monitor.Record("TEST/ASSET", AnomalyInsufficientDepth)

			// Wait for drain worker
			time.Sleep(20 * time.Millisecond)

			So(monitor.Count("TEST/ASSET"), ShouldEqual, 2)
			So(monitor.Total(), ShouldEqual, 2)
			So(monitor.Health("TEST/ASSET"), ShouldBeLessThan, 1.0)
		})
	})
}
