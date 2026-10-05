package hawkes_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/hawkes"
)

func TestHawkesTradeMetrics(t *testing.T) {
	Convey("Hawkes arrival-dynamics instrument computes principled point-process metrics", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner(4096)

		instrument := hawkes.NewSignal(ctx, arena)
		instrument.Transition(nmruntime.READY)

		now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		Convey("Computes exact event counts, side fractions, and arrival rates across ticks", func() {
			var buyCount, sellCount float64
			origin := now

			for step := 0; step < 20; step++ {
				prior := arena.NewMeasurement("ingress")
				prior.Label = "BTC/USD"
				prior.SeqIdx = int64(step + 1)
				prior.At = now.Add(time.Duration(step) * 50 * time.Millisecond)
				prior.From = prior.At

				prior.SetMetric("price", data.NewMetric[float64](
					"price",
					data.UnitPrice,
					data.TimescaleInstantaneous,
					50000.0,
					1.0,
				).Write(50000.0))
				prior.SetMetric("qty", data.NewMetric[float64](
					"qty",
					data.UnitQuantity,
					data.TimescaleInstantaneous,
					0.0,
					1.0,
				).Write(1.0))
				prior.SetProvenance("channel", "trade")

				side := "buy"
				if step%3 == 0 {
					side = "sell"
					sellCount++
				} else {
					buyCount++
				}
				prior.SetProvenance("side", side)

				res := instrument.Step(prior)
				So(res, ShouldNotBeNil)
				So(res.Err, ShouldBeNil)

				totalCount := float64(step + 1)
				So(res.GetMetric("event_count").Raw, ShouldEqual, totalCount)
				So(res.GetMetric("event_count:buy").Raw, ShouldEqual, buyCount)
				So(res.GetMetric("event_count:sell").Raw, ShouldEqual, sellCount)

				fracBuy := res.GetMetric("event_fraction:buy").Raw
				fracSell := res.GetMetric("event_fraction:sell").Raw
				So(fracBuy, ShouldAlmostEqual, buyCount/totalCount, 1e-9)
				So(fracSell, ShouldAlmostEqual, sellCount/totalCount, 1e-9)
				So(fracBuy+fracSell, ShouldAlmostEqual, 1.0, 1e-9)

				span := prior.At.Sub(origin).Seconds()
				if span > 0 {
					expectedBuyRate := buyCount / span
					expectedSellRate := sellCount / span
					expectedTotalRate := (buyCount + sellCount) / span

					So(res.GetMetric("arrival_rate:buy").Raw, ShouldAlmostEqual, expectedBuyRate, 1e-6)
					So(res.GetMetric("arrival_rate:sell").Raw, ShouldAlmostEqual, expectedSellRate, 1e-6)
					So(res.GetMetric("arrival_rate").Raw, ShouldAlmostEqual, expectedTotalRate, 1e-6)
				}

				So(res.Maturity, ShouldBeGreaterThanOrEqualTo, 0)
				So(res.Maturity, ShouldBeLessThanOrEqualTo, 1.0)
			}
		})

		Convey("Enforces gate validation: drops invalid trade side with domain error", func() {
			prior := arena.NewMeasurement("ingress")
			prior.Label = "BTC/USD"
			prior.SeqIdx = 100
			prior.At = now
			prior.From = now
			prior.SetMetric("price", data.NewMetric[float64](
				"price",
				data.UnitPrice,
				data.TimescaleInstantaneous,
				50000.0,
				1.0,
			).Write(50000.0))
			prior.SetMetric("qty", data.NewMetric[float64](
				"qty",
				data.UnitQuantity,
				data.TimescaleInstantaneous,
				0.0,
				1.0,
			).Write(1.0))
			prior.SetProvenance("channel", "trade")
			prior.SetProvenance("side", "neutral") // invalid side

			res := instrument.Step(prior)
			So(res, ShouldNotBeNil)
			So(res.Err, ShouldNotBeNil)
		})

		Convey("Drops non-trade channel measurements without processing", func() {
			prior := arena.NewMeasurement("ingress")
			prior.Label = "BTC/USD"
			prior.SeqIdx = 101
			prior.At = now
			prior.From = now
			prior.SetProvenance("channel", "book")
			prior.SetProvenance("side", "buy")
			prior.SetMetric("price", data.NewMetric[float64](
				"price",
				data.UnitPrice,
				data.TimescaleInstantaneous,
				50000.0,
				1.0,
			).Write(50000.0))
			prior.SetMetric("qty", data.NewMetric[float64](
				"qty",
				data.UnitQuantity,
				data.TimescaleInstantaneous,
				0.0,
				1.0,
			).Write(1.0))

			res := instrument.Step(prior)
			So(res, ShouldBeNil)
		})
	})
}
