package leadlag

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSignalNewSignal(t *testing.T) {
	Convey("Given a new lead-lag signal", t, func() {
		signal := NewSignal(context.Background(), nil)

		Convey("it reports its name and closes cleanly", func() {
			So(signal.Name(), ShouldEqual, "leadlag")
			So(signal.Error(), ShouldBeNil)
			So(signal.Close(), ShouldBeNil)
		})

		Convey("it steps one ticker into one measurement", func() {
			measurement := signal.Step(ticker("BTC/USD", 100.0, timestamp(1)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["last_price"].Raw, ShouldEqual, 100.0)
		})
	})
}
