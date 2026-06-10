package logic

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
)

func TestMeasurementPublishable(t *testing.T) {
	Convey("Given measurement publishability rules", t, func() {
		complete := Measurement{
			Source:     SourceFluid,
			Symbol:     "BTC/USD",
			ObservedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		}

		Convey("Publishable should accept complete measurements", func() {
			So(complete.Publishable(), ShouldBeTrue)
		})

		Convey("Publishable should reject warmup stubs", func() {
			stub := Measurement{Symbol: "BTC/USD"}

			So(stub.Publishable(), ShouldBeFalse)
		})

		Convey("Publish should not send incomplete measurements", func() {
			ctx := context.Background()
			pool := qpool.NewQ[any](ctx, 2, 8, nil)
			bus := internal.NewBus(ctx, pool, []internal.Channel{internal.ChannelMeasurements}, []internal.Subscription{internal.Subscribe(internal.ChannelMeasurements, "test-measurements")})

			So(complete.Publish(bus), ShouldBeNil)

			row, pollErr := bus.Poll("measurements")

			So(pollErr, ShouldBeNil)
			So(row, ShouldNotBeNil)

			stub := Measurement{Symbol: "ETH/USD"}

			So(stub.Publish(bus), ShouldBeNil)

			secondRow, secondPollErr := bus.Poll("measurements")

			So(secondPollErr, ShouldBeNil)
			So(secondRow, ShouldBeNil)
		})
	})
}
