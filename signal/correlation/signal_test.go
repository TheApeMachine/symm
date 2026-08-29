package correlation

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestSignalNewSignal(t *testing.T) {
	Convey("Given a new correlation signal", t, func() {
		signal := NewSignal(context.Background())

		Convey("it reports its name and closes cleanly", func() {
			So(signal.Name(), ShouldEqual, "correlation")
			So(signal.Error(), ShouldBeNil)
			So(signal.Close(), ShouldBeNil)
		})

		Convey("it steps one ticker into one measurement", func() {
			envelope := types.NewEnvelope(types.EnvelopeTicker)
			envelope.TickerData = ticker("BTC/USD", 100.0, timestamp(1))

			result := signal.Step(envelope)

			So(result.Correlation, ShouldNotBeNil)
			So(result.Correlation.Err, ShouldBeNil)
			So(result.Correlation.Metrics["last_price"].Raw, ShouldEqual, 100.0)
		})
	})
}
