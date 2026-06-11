package logic

import (
	"context"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestMeasurementPublishable(t *testing.T) {
	Convey("Given measurement publishability rules", t, func() {
		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		row, rowErr := krakenmarket.NewSymbolRow(
			"BTC/USD",
			42000,
			0.01,
			42000,
			1,
			eventAt,
		)

		So(rowErr, ShouldBeNil)

		complete := Measurement{
			Source:     SourceFluid,
			Symbol:     "BTC/USD",
			Price:      42000,
			Strength:   0.5,
			Volume:     100,
			Spread:     1,
			Elapsed:    1,
			Confidence: 0.8,
			Surprise:   1.2,
			ObservedAt: eventAt,
			Market:     *row,
		}

		Convey("Publishable should accept complete measurements", func() {
			So(complete.Publishable(), ShouldBeTrue)
		})

		Convey("Publishable should reject incomplete measurements", func() {
			incomplete := Measurement{Symbol: "BTC/USD"}

			So(incomplete.Publishable(), ShouldBeFalse)
		})

		Convey("Publish should reject non-finite floats", func() {
			ctx := context.Background()
			pool := qpool.NewQ[any](ctx, 2, 8, nil)
			bus := internal.NewBus(
				ctx,
				pool,
				[]internal.Channel{internal.ChannelMeasurements},
				[]internal.Subscription{
					internal.Subscribe(internal.ChannelMeasurements, "test-measurements"),
				},
			)

			invalid := Measurement{
				Source:     SourceDepthFlow,
				Symbol:     "ETH/USD",
				Price:      1600,
				Strength:   math.NaN(),
				Volume:     100,
				Spread:     1,
				Elapsed:    1,
				Confidence: 0.8,
				Surprise:   1.2,
				ObservedAt: eventAt,
				Market:     *row,
			}

			So(invalid.Publish(bus), ShouldNotBeNil)

			polled, pollErr := bus.Poll("measurements")

			So(pollErr, ShouldBeNil)
			So(polled, ShouldBeNil)
		})

		Convey("Publish should reject incomplete measurements", func() {
			ctx := context.Background()
			pool := qpool.NewQ[any](ctx, 2, 8, nil)
			bus := internal.NewBus(
				ctx,
				pool,
				[]internal.Channel{internal.ChannelMeasurements},
				[]internal.Subscription{
					internal.Subscribe(internal.ChannelMeasurements, "test-measurements"),
				},
			)

			So(complete.Publish(bus), ShouldBeNil)

			polled, pollErr := bus.Poll("measurements")

			So(pollErr, ShouldBeNil)
			So(polled, ShouldNotBeNil)

			incomplete := Measurement{Symbol: "ETH/USD"}

			So(incomplete.Publish(bus), ShouldNotBeNil)

			secondRow, secondPollErr := bus.Poll("measurements")

			So(secondPollErr, ShouldBeNil)
			So(secondRow, ShouldBeNil)
		})
	})
}
