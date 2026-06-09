package prediction

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
)

func TestChartApply(t *testing.T) {
	Convey("Given a chart bound to the ui bus", t, func() {
		Convey("It should publish mean forecast at the latest target horizon", func() {
			ctx := context.Background()
			pool := qpool.NewQ[any](ctx, 2, 8, nil)
			publisher := internal.NewBus(ctx, pool, []string{"ui"}, nil)
			subscriber := internal.NewBus(ctx, pool, nil, []string{"ui"})
			chart := NewChart(publisher, time.Minute)

			So(chart.Apply("BTC/EUR", ChartEvents{
				ForecastTarget: 1_710_000_060,
				Forecast:       0.02,
				HasForecast:    true,
			}), ShouldBeNil)
			So(chart.Apply("ETH/EUR", ChartEvents{
				ForecastTarget: 1_710_000_120,
				Forecast:       0.04,
				HasForecast:    true,
			}), ShouldBeNil)

			var lastPayload map[string]any

			for range 2 {
				frame, err := subscriber.Receive("ui")

				So(err, ShouldBeNil)
				So(frame, ShouldNotBeNil)
				So(frame.Type, ShouldEqual, "prediction")

				payload, ok := frame.Value.(map[string]any)

				So(ok, ShouldBeTrue)
				lastPayload = payload
			}

			So(lastPayload["chart"], ShouldEqual, "prediction")
			So(lastPayload["kind"], ShouldEqual, "prediction")
			So(lastPayload["x"], ShouldEqual, float64(1_710_000_120))
			So(lastPayload["value"], ShouldEqual, 0.03)
			So(lastPayload["horizon"], ShouldEqual, 60.0)
		})

		Convey("It should publish mean actual and error at settlement time", func() {
			ctx := context.Background()
			pool := qpool.NewQ[any](ctx, 2, 8, nil)
			publisher := internal.NewBus(ctx, pool, []string{"ui"}, nil)
			subscriber := internal.NewBus(ctx, pool, nil, []string{"ui"})
			chart := NewChart(publisher, time.Minute)

			So(chart.Apply("BTC/EUR", ChartEvents{
				Settlements: []ChartSettlement{{
					TargetUnix: 1_710_000_000,
					Forecast:   0.05,
					Actual:     0.02,
				}},
			}), ShouldBeNil)
			So(chart.Apply("ETH/EUR", ChartEvents{
				Settlements: []ChartSettlement{{
					TargetUnix: 1_710_000_000,
					Forecast:   0.06,
					Actual:     0.04,
				}},
			}), ShouldBeNil)

			var (
				predictionPayload map[string]any
				actualPayload     map[string]any
				errorPayload      map[string]any
			)

			for range 6 {
				frame, receiveErr := subscriber.Receive("ui")

				So(receiveErr, ShouldBeNil)

				payload, payloadOK := frame.Value.(map[string]any)

				So(payloadOK, ShouldBeTrue)

				switch payload["kind"] {
				case "prediction":
					predictionPayload = payload
				case "actual":
					actualPayload = payload
				case "error":
					errorPayload = payload
				}
			}

			So(predictionPayload["kind"], ShouldEqual, "prediction")
			So(predictionPayload["x"], ShouldEqual, float64(1_710_000_000))
			So(predictionPayload["value"], ShouldEqual, 0.055)
			So(actualPayload["kind"], ShouldEqual, "actual")
			So(actualPayload["x"], ShouldEqual, float64(1_710_000_000))
			So(actualPayload["value"], ShouldEqual, 0.03)
			So(errorPayload["kind"], ShouldEqual, "error")
			So(errorPayload["x"], ShouldEqual, float64(1_710_000_000))
			So(errorPayload["value"], ShouldEqual, 0.025)
		})

		Convey("It should align error with the plotted mean lines", func() {
			ctx := context.Background()
			pool := qpool.NewQ[any](ctx, 2, 8, nil)
			publisher := internal.NewBus(ctx, pool, []string{"ui"}, nil)
			subscriber := internal.NewBus(ctx, pool, nil, []string{"ui"})
			chart := NewChart(publisher, time.Minute)

			So(chart.Apply("BTC/EUR", ChartEvents{
				Settlements: []ChartSettlement{{
					TargetUnix: 1_710_000_500,
					Forecast:   0.5,
					Actual:     0.0,
				}},
			}), ShouldBeNil)
			So(chart.Apply("ETH/EUR", ChartEvents{
				Settlements: []ChartSettlement{{
					TargetUnix: 1_710_000_500,
					Forecast:   -0.5,
					Actual:     0.0,
				}},
			}), ShouldBeNil)

			var errorPayload map[string]any

			for range 6 {
				frame, receiveErr := subscriber.Receive("ui")

				So(receiveErr, ShouldBeNil)

				payload, payloadOK := frame.Value.(map[string]any)

				So(payloadOK, ShouldBeTrue)

				if payload["kind"] == "error" {
					errorPayload = payload
				}
			}

			So(errorPayload["value"], ShouldEqual, 0.0)
		})

		Convey("It should publish one live forecast per target second", func() {
			ctx := context.Background()
			pool := qpool.NewQ[any](ctx, 2, 8, nil)
			publisher := internal.NewBus(ctx, pool, []string{"ui"}, nil)
			subscriber := internal.NewBus(ctx, pool, nil, []string{"ui"})
			chart := NewChart(publisher, time.Minute)

			target := float64(1_710_000_120)

			So(chart.Apply("BTC/EUR", ChartEvents{
				ForecastTarget: target,
				Forecast:       0.02,
				HasForecast:    true,
			}), ShouldBeNil)
			So(chart.Apply("ETH/EUR", ChartEvents{
				ForecastTarget: target,
				Forecast:       0.04,
				HasForecast:    true,
			}), ShouldBeNil)

			var lastPayload map[string]any

			for range 2 {
				frame, err := subscriber.Receive("ui")

				So(err, ShouldBeNil)

				payload, ok := frame.Value.(map[string]any)

				So(ok, ShouldBeTrue)
				lastPayload = payload
			}

			So(lastPayload["x"], ShouldEqual, target)
			So(lastPayload["value"], ShouldEqual, 0.03)

			thirdFrame, thirdErr := subscriber.Poll("ui")

			So(thirdErr, ShouldBeNil)
			So(thirdFrame, ShouldBeNil)
		})
	})
}

func BenchmarkChartApply(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ[any](ctx, 2, 8, nil)
	bus := internal.NewBus(ctx, pool, []string{"ui"}, nil)
	chart := NewChart(bus, time.Minute)

	events := ChartEvents{
		ForecastTarget: float64(time.Now().Add(time.Minute).Unix()),
		Forecast:       0.02,
		HasForecast:    true,
	}

	b.ReportAllocs()

	for b.Loop() {
		if err := chart.Apply("BTC/EUR", events); err != nil {
			b.Fatal(err)
		}
	}
}
