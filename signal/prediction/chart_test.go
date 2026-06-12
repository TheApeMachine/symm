package prediction

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
)

func init() {
	viper.Set("story.prediction.interval", 0)
}

func receivePredictionFrames(
	subscriber *internal.Bus,
	count int,
) []map[string]any {
	frames := make([]map[string]any, 0, count)

	for range count {
		frame, err := subscriber.Receive(internal.ChannelUI)

		So(err, ShouldBeNil)
		So(frame, ShouldNotBeNil)

		payload, ok := frame.Value.(map[string]any)

		So(ok, ShouldBeTrue)
		frames = append(frames, payload)
	}

	return frames
}

func TestChartApply(t *testing.T) {
	Convey("Given a chart bound to the ui bus", t, func() {
		Convey("It should publish mean forecast at each maturity target second", func() {
			ctx := context.Background()
			pool := qpool.NewQ[any](ctx, 2, 8, nil)
			publisher := internal.NewBus(ctx, pool, []internal.Channel{internal.ChannelUI}, []internal.Subscription{internal.Subscribe(internal.ChannelUI, "test-ui")})
			subscriber := internal.NewBus(ctx, pool, nil, []internal.Subscription{internal.Subscribe(internal.ChannelUI, "test-ui-sub")})
			chart := NewChart(publisher, time.Minute)
			eventAt := time.Unix(1_710_000_000, 0)

			So(chart.Apply("BTC/EUR", ChartEvents{
				EventAt:        eventAt,
				ForecastTarget: 1_710_000_060,
				Forecast:       0.02,
				HasForecast:    true,
			}), ShouldBeNil)
			So(chart.Apply("ETH/EUR", ChartEvents{
				EventAt:        eventAt,
				ForecastTarget: 1_710_000_120,
				Forecast:       0.04,
				HasForecast:    true,
			}), ShouldBeNil)

			frames := receivePredictionFrames(subscriber, 2)
			lastPayload := frames[len(frames)-1]

			So(lastPayload["type"], ShouldEqual, "prediction")
			So(lastPayload["chart"], ShouldEqual, "prediction")
			So(lastPayload["kind"], ShouldEqual, "prediction")
			So(lastPayload["x"], ShouldEqual, float64(1_710_000_120))
			So(lastPayload["value"], ShouldEqual, 0.04)
			So(lastPayload["horizon"], ShouldEqual, 60.0)
			So(lastPayload["samples"], ShouldEqual, 1)
		})

		Convey("It should average symbols sharing the same maturity target", func() {
			ctx := context.Background()
			pool := qpool.NewQ[any](ctx, 2, 8, nil)
			publisher := internal.NewBus(ctx, pool, []internal.Channel{internal.ChannelUI}, []internal.Subscription{internal.Subscribe(internal.ChannelUI, "test-ui")})
			subscriber := internal.NewBus(ctx, pool, nil, []internal.Subscription{internal.Subscribe(internal.ChannelUI, "test-ui-sub")})
			chart := NewChart(publisher, time.Minute)

			target := float64(1_710_000_120)
			eventAt := time.Unix(1_710_000_060, 0)

			So(chart.Apply("BTC/EUR", ChartEvents{
				EventAt:        eventAt,
				ForecastTarget: target,
				Forecast:       0.02,
				HasForecast:    true,
			}), ShouldBeNil)
			So(chart.Apply("ETH/EUR", ChartEvents{
				EventAt:        eventAt,
				ForecastTarget: target,
				Forecast:       0.04,
				HasForecast:    true,
			}), ShouldBeNil)

			frames := receivePredictionFrames(subscriber, 2)
			lastPayload := frames[len(frames)-1]

			So(lastPayload["x"], ShouldEqual, target)
			So(lastPayload["value"], ShouldEqual, 0.03)
			So(lastPayload["samples"], ShouldEqual, 2)
		})

		Convey("It should publish settlement trio after the maturity second closes", func() {
			ctx := context.Background()
			pool := qpool.NewQ[any](ctx, 2, 8, nil)
			publisher := internal.NewBus(ctx, pool, []internal.Channel{internal.ChannelUI}, []internal.Subscription{internal.Subscribe(internal.ChannelUI, "test-ui")})
			subscriber := internal.NewBus(ctx, pool, nil, []internal.Subscription{internal.Subscribe(internal.ChannelUI, "test-ui-sub")})
			chart := NewChart(publisher, time.Minute)

			target := int64(1_710_000_000)

			So(chart.Apply("BTC/EUR", ChartEvents{
				EventAt: time.Unix(target, 0),
				Settlements: []ChartSettlement{{
					TargetUnix: float64(target),
					Forecast:   0.05,
					Actual:     0.02,
				}},
			}), ShouldBeNil)
			So(chart.Apply("ETH/EUR", ChartEvents{
				EventAt: time.Unix(target, 0),
				Settlements: []ChartSettlement{{
					TargetUnix: float64(target),
					Forecast:   0.06,
					Actual:     0.04,
				}},
			}), ShouldBeNil)

			_, pollErr := subscriber.Poll(internal.ChannelUI)

			So(pollErr, ShouldBeNil)

			So(chart.Apply("BTC/EUR", ChartEvents{
				EventAt: time.Unix(target+1, 0),
			}), ShouldBeNil)

			frames := receivePredictionFrames(subscriber, 3)

			var (
				forecastPayload map[string]any
				actualPayload   map[string]any
				errorPayload    map[string]any
			)

			for _, payload := range frames {
				switch payload["kind"] {
				case "prediction":
					forecastPayload = payload
				case "actual":
					actualPayload = payload
				case "error":
					errorPayload = payload
				}
			}

			So(forecastPayload["x"], ShouldEqual, float64(target))
			So(forecastPayload["value"], ShouldEqual, 0.055)
			So(actualPayload["kind"], ShouldEqual, "actual")
			So(actualPayload["x"], ShouldEqual, float64(target))
			So(actualPayload["value"], ShouldEqual, 0.03)
			So(errorPayload["kind"], ShouldEqual, "error")
			So(errorPayload["x"], ShouldEqual, float64(target))
			So(errorPayload["value"], ShouldEqual, 0.025)
		})

		Convey("It should keep historical forecasts when a symbol rolls forward", func() {
			ctx := context.Background()
			pool := qpool.NewQ[any](ctx, 2, 8, nil)
			publisher := internal.NewBus(ctx, pool, []internal.Channel{internal.ChannelUI}, []internal.Subscription{internal.Subscribe(internal.ChannelUI, "test-ui")})
			subscriber := internal.NewBus(ctx, pool, nil, []internal.Subscription{internal.Subscribe(internal.ChannelUI, "test-ui-sub")})
			chart := NewChart(publisher, time.Minute)

			firstTarget := float64(1_710_000_060)
			secondTarget := float64(1_710_000_120)
			eventAt := time.Unix(1_710_000_000, 0)

			So(chart.Apply("BTC/EUR", ChartEvents{
				EventAt:        eventAt,
				ForecastTarget: firstTarget,
				Forecast:       0.02,
				HasForecast:    true,
			}), ShouldBeNil)
			_, _ = subscriber.Receive(internal.ChannelUI)

			So(chart.Apply("BTC/EUR", ChartEvents{
				EventAt:        eventAt,
				ForecastTarget: secondTarget,
				Forecast:       0.10,
				HasForecast:    true,
			}), ShouldBeNil)

			frame, err := subscriber.Receive(internal.ChannelUI)

			So(err, ShouldBeNil)

			payload, ok := frame.Value.(map[string]any)

			So(ok, ShouldBeTrue)
			So(payload["x"], ShouldEqual, secondTarget)
			So(payload["value"], ShouldEqual, 0.10)

			firstBucket := chart.forecastBuckets[int64(firstTarget)]

			So(firstBucket, ShouldNotBeNil)
			So(firstBucket.forecasts["BTC/EUR"], ShouldEqual, 0.02)
		})

		Convey("It should align error with the plotted mean lines", func() {
			ctx := context.Background()
			pool := qpool.NewQ[any](ctx, 2, 8, nil)
			publisher := internal.NewBus(ctx, pool, []internal.Channel{internal.ChannelUI}, []internal.Subscription{internal.Subscribe(internal.ChannelUI, "test-ui")})
			subscriber := internal.NewBus(ctx, pool, nil, []internal.Subscription{internal.Subscribe(internal.ChannelUI, "test-ui-sub")})
			chart := NewChart(publisher, time.Minute)

			target := int64(1_710_000_500)

			So(chart.Apply("BTC/EUR", ChartEvents{
				EventAt: time.Unix(target, 0),
				Settlements: []ChartSettlement{{
					TargetUnix: float64(target),
					Forecast:   0.5,
					Actual:     0.0,
				}},
			}), ShouldBeNil)
			So(chart.Apply("ETH/EUR", ChartEvents{
				EventAt: time.Unix(target, 0),
				Settlements: []ChartSettlement{{
					TargetUnix: float64(target),
					Forecast:   -0.5,
					Actual:     0.0,
				}},
			}), ShouldBeNil)
			So(chart.Apply("BTC/EUR", ChartEvents{
				EventAt: time.Unix(target+1, 0),
			}), ShouldBeNil)

			frames := receivePredictionFrames(subscriber, 3)

			var errorPayload map[string]any

			for _, payload := range frames {
				if payload["kind"] == "error" {
					errorPayload = payload
				}
			}

			So(errorPayload["value"], ShouldEqual, 0.0)
		})

		Convey("It should republish when the cross-section mean changes at the same target", func() {
			ctx := context.Background()
			pool := qpool.NewQ[any](ctx, 2, 8, nil)
			publisher := internal.NewBus(ctx, pool, []internal.Channel{internal.ChannelUI}, []internal.Subscription{internal.Subscribe(internal.ChannelUI, "test-ui")})
			subscriber := internal.NewBus(ctx, pool, nil, []internal.Subscription{internal.Subscribe(internal.ChannelUI, "test-ui-sub")})
			chart := NewChart(publisher, time.Minute)

			target := float64(1_710_000_120)
			eventAt := time.Unix(1_710_000_060, 0)

			So(chart.Apply("BTC/EUR", ChartEvents{
				EventAt:        eventAt,
				ForecastTarget: target,
				Forecast:       0.02,
				HasForecast:    true,
			}), ShouldBeNil)
			So(chart.Apply("ETH/EUR", ChartEvents{
				EventAt:        eventAt,
				ForecastTarget: target,
				Forecast:       0.04,
				HasForecast:    true,
			}), ShouldBeNil)

			receivePredictionFrames(subscriber, 2)

			So(chart.Apply("BTC/EUR", ChartEvents{
				EventAt:        eventAt,
				ForecastTarget: target,
				Forecast:       0.10,
				HasForecast:    true,
			}), ShouldBeNil)

			frame, err := subscriber.Receive(internal.ChannelUI)

			So(err, ShouldBeNil)

			payload, ok := frame.Value.(map[string]any)

			So(ok, ShouldBeTrue)
			So(payload["x"], ShouldEqual, target)
			So(payload["value"], ShouldEqual, 0.07)
			So(payload["samples"], ShouldEqual, 2)
		})

		Convey("It should throttle forecast frames to the configured interval", func() {
			viper.Set("story.prediction.interval", time.Second)

			ctx := context.Background()
			pool := qpool.NewQ[any](ctx, 2, 8, nil)
			publisher := internal.NewBus(ctx, pool, []internal.Channel{internal.ChannelUI}, []internal.Subscription{internal.Subscribe(internal.ChannelUI, "test-ui")})
			subscriber := internal.NewBus(ctx, pool, nil, []internal.Subscription{internal.Subscribe(internal.ChannelUI, "test-ui-sub")})
			chart := NewChart(publisher, time.Minute)
			firstAt := time.Unix(1_710_000_000, 0)
			secondAt := firstAt.Add(100 * time.Millisecond)

			So(chart.Apply("BTC/EUR", ChartEvents{
				EventAt:        firstAt,
				ForecastTarget: 1_710_000_060,
				Forecast:       0.02,
				HasForecast:    true,
			}), ShouldBeNil)
			So(chart.Apply("ETH/EUR", ChartEvents{
				EventAt:        secondAt,
				ForecastTarget: 1_710_000_120,
				Forecast:       0.04,
				HasForecast:    true,
			}), ShouldBeNil)

			frames := receivePredictionFrames(subscriber, 1)

			So(frames[0]["kind"], ShouldEqual, "prediction")

			viper.Set("story.prediction.interval", 0)
		})
	})
}

func BenchmarkChartApply(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ[any](ctx, 2, 8, nil)
	bus := internal.NewBus(ctx, pool, []internal.Channel{internal.ChannelUI}, []internal.Subscription{internal.Subscribe(internal.ChannelUI, "test-ui")})
	chart := NewChart(bus, time.Minute)

	events := ChartEvents{
		EventAt:        time.Now(),
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
