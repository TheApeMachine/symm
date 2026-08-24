package hawkes

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSignalStep(t *testing.T) {
	Convey("Given a Hawkes signal", t, func() {
		signal := NewSignal(context.Background())

		Convey("Name reports the signal identity", func() {
			So(signal.Name(), ShouldEqual, "hawkes")
		})

		Convey("Step delegates to the trade entity", func() {
			measurement := signal.Step(hawkesTrade("BTC/USD", "buy", time.Unix(1000, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["event_count"].Raw, ShouldEqual, 1.0)
		})

		Convey("Close releases without error", func() {
			So(signal.Close(), ShouldBeNil)
		})
	})
}
