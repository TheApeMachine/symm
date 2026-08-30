package cvd

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestSignalStep(t *testing.T) {
	Convey("Given a CVD signal without a workspace", t, func() {
		signal := NewSignal(context.Background(), nil)

		Convey("Name reports the signal identity", func() {
			So(signal.Name(), ShouldEqual, "cvd")
		})

		Convey("Step delegates to the trade entity", func() {
			envelope := types.NewEnvelope(types.EnvelopeTrade)
			envelope.TradeData = cvdTrade("BTC/USD", "buy", 100, 2, time.Unix(1000, 0))

			result := signal.Step(envelope)

			So(result.CVD, ShouldNotBeNil)
			So(result.CVD.Err, ShouldBeNil)
			So(result.CVD.Metrics["trade_count"].Raw, ShouldEqual, 1.0)
		})

		Convey("Close releases without error", func() {
			So(signal.Close(), ShouldBeNil)
		})
	})
}
