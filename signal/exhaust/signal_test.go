package exhaust

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewSignal(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)

	Convey("Given a signal", t, func() {
		signal := NewSignal(context.Background())

		Convey("Name reports the signal identity", func() {
			So(signal.Name(), ShouldEqual, "exhaust")
		})

		Convey("Error is nil on a healthy signal", func() {
			So(signal.Error(), ShouldBeNil)
		})

		Convey("Step delegates to the Ticker entity", func() {
			measurement := signal.Step(ticker("BTC/USD", 99, 101, 2, 2, now))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics, ShouldNotBeEmpty)
		})

		Convey("Close releases the signal", func() {
			So(signal.Close(), ShouldBeNil)
		})
	})
}
