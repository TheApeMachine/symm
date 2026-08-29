package hawkes

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestSignalStep(t *testing.T) {
	Convey("Given a Hawkes signal", t, func() {
		signal := NewSignal(context.Background())

		Convey("Name reports the signal identity", func() {
			So(signal.Name(), ShouldEqual, "hawkes")
		})

		Convey("Step delegates to the trade entity", func() {
			envelope := types.NewEnvelope(types.EnvelopeTrade)
			envelope.TradeData = hawkesTrade("BTC/USD", "buy", time.Unix(1000, 0))

			result := signal.Step(envelope)

			So(result.Hawkes, ShouldNotBeNil)
			So(result.Hawkes.Err, ShouldBeNil)
			So(result.Hawkes.Metrics["event_count"].Raw, ShouldEqual, 1.0)
		})

		Convey("Close releases without error", func() {
			So(signal.Close(), ShouldBeNil)
		})
	})
}
