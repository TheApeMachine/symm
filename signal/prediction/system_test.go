package prediction

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

func TestSystemPublishChartOnTrade(t *testing.T) {
	Convey("Given a prediction system with a bound chart", t, func() {
		viper.Set("signals.prediction.measurements_capacity", 16)
		viper.Set("story.prediction.horizon", time.Minute)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 2, 4, nil)
		defer pool.Close()

		system := NewSystem(ctx, pool)

		So(system, ShouldNotBeNil)
		So(system.chart, ShouldNotBeNil)

		uiBus := internal.NewBus(ctx, pool, nil, []internal.Subscription{internal.Subscribe(internal.ChannelUI, "test-ui")})
		receiveDone := make(chan map[string]any, 1)

		go func() {
			for {
				row, receiveErr := uiBus.Receive(internal.ChannelUI)

				if receiveErr != nil {
					if ctx.Err() != nil {
						return
					}

					continue
				}

				if row == nil {
					continue
				}

				payload, decodeErr := qpool.ArtifactValue[map[string]any](row)

				if decodeErr != nil || payload["chart"] != "prediction" {
					continue
				}

				receiveDone <- payload
			}
		}()

		Convey("It should publish prediction chart frames on the ui bus", func() {
			applyErr := system.chart.Apply("BTC/USD", ChartEvents{
				EventAt:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				ForecastTarget: float64(time.Date(2024, 1, 1, 0, 1, 0, 0, time.UTC).Unix()),
				Forecast:       0.02,
				HasForecast:    true,
			})

			So(applyErr, ShouldBeNil)

			var frame map[string]any

			select {
			case frame = <-receiveDone:
			case <-time.After(2 * time.Second):
				So("ui prediction frame", ShouldEqual, "received")
			}

			So(frame, ShouldNotBeNil)
			So(frame["kind"], ShouldEqual, "prediction")
			So(frame["horizon"], ShouldEqual, 60.0)
		})

		Convey("It should publish trade-driven chart frames once horizon scale is calibrated", func() {
			signal := NewSignal(
				"BTC/USD",
				logic.NewEntity(logic.EntityTrade),
				system.chart,
			)
			signal.realizedMagnitudeEMA = 0.01
			signal.features[0] = 0.5
			coefficients := signal.learner.Coefficients()
			coefficients[1] = 0.05
			So(signal.learner.SetCoefficients(coefficients), ShouldBeNil)

			for index := range 4 {
				signal.Record(&krakenmarket.TradeUpdate{
					Symbol:    "BTC/USD",
					Price:     50000 + float64(index)*0.01,
					Qty:       1,
					Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				})
			}

			_, measureErr := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 1, 0, time.UTC))

			So(measureErr, ShouldBeNil)
			So(signal.DrainChartEvents().HasForecast, ShouldBeFalse)

			var frame map[string]any

			select {
			case frame = <-receiveDone:
			case <-time.After(2 * time.Second):
				So("trade-driven ui prediction frame", ShouldEqual, "received")
			}

			So(frame, ShouldNotBeNil)
			So(frame["kind"], ShouldEqual, "prediction")
		})
	})
}

func BenchmarkSystemPublishChartOnTrade(b *testing.B) {
	viper.Set("signals.prediction.measurements_capacity", 16)
	viper.Set("story.prediction.horizon", time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := qpool.NewQ[any](ctx, 2, 4, nil)
	defer pool.Close()

	system := NewSystem(ctx, pool)

	if system == nil {
		b.Fatal("system is nil")
	}

	uiBus := internal.NewBus(ctx, pool, nil, []internal.Subscription{internal.Subscribe(internal.ChannelUI, "test-ui")})

	go func() {
		for {
			row, receiveErr := uiBus.Receive(internal.ChannelUI)

			if receiveErr != nil {
				if ctx.Err() != nil {
					return
				}

				continue
			}

			_ = row
		}
	}()

	signal := NewSignal(
		"BTC/USD",
		logic.NewEntity(logic.EntityTrade),
		system.chart,
	)
	signal.realizedMagnitudeEMA = 0.01

	tradeAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	b.ReportAllocs()

	for b.Loop() {
		signal.Record(&krakenmarket.TradeUpdate{
			Symbol:    "BTC/USD",
			Price:     50000 + float64(b.N),
			Qty:       1,
			Timestamp: tradeAt,
		})

		if _, err := signal.Measure(nil, tradeAt); err != nil {
			b.Fatal(err)
		}
	}
}
