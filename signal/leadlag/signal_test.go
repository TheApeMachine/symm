package leadlag

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestSignalNewSignal(t *testing.T) {
	Convey("Given a new lead-lag signal", t, func() {
		signal := NewSignal(context.Background())

		Convey("it reports its name and closes cleanly", func() {
			So(signal.Name(), ShouldEqual, "leadlag")
			So(signal.Error(), ShouldBeNil)
			So(signal.Close(), ShouldBeNil)
		})

		Convey("it steps one ticker into one measurement", func() {
			envelope := types.NewEnvelope(types.EnvelopeTicker)
			envelope.TickerData = ticker("BTC/USD", 100.0, timestamp(1))

			result := signal.Step(envelope)

			So(result.LeadLag, ShouldNotBeNil)
			So(result.LeadLag.Err, ShouldBeNil)
			So(result.LeadLag.Metrics["last_price"].Raw, ShouldEqual, 100.0)
		})
	})
}
