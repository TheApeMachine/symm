package cvd

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestSignalStep(t *testing.T) {
	Convey("Given a CVD signal without a workspace", t, func() {
		signal := NewSignal(context.Background())

		Convey("Name reports the signal identity", func() {
			So(signal.Name(), ShouldEqual, "cvd")
		})

		Convey("Step delegates to the trade entity", func() {
			measurement := signal.Step(cvdTrade("BTC/USD", "buy", 100, 2, time.Unix(1000, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["trade_count"].Raw, ShouldEqual, 1.0)
		})

		Convey("Close releases without error", func() {
			So(signal.Close(), ShouldBeNil)
		})
	})

	Convey("Given a CVD signal with a workspace", t, func() {
		workspace := runtime.NewWorkspace(nil)
		workspace.Share("book", cvdBook(99, 101), "BTC/USD")
		signal := NewSignal(context.Background(), workspace)

		Convey("the shared book supplies the response-price metrics", func() {
			signal.Step(cvdTrade("BTC/USD", "buy", 100, 2, time.Unix(1000, 0)))
			measurement := signal.Step(cvdTrade("BTC/USD", "sell", 100, 1, time.Unix(1001, 0)))

			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["midpoint_log_return"].Raw, ShouldEqual, 0.0)
		})

		Reset(func() {
			_ = signal.Close()
		})
	})
}
