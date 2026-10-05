package cvd_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/cvd"
)

func TestCVDSignalMetrics(t *testing.T) {
	Convey("CVD signal instrument calculates mathematically rigorous cumulative volume and flow metrics", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner(4096)
		instrument := cvd.NewSignal(ctx, arena)
		instrument.Transition(nmruntime.READY)

		now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		origin := now

		Convey("Accumulates exact quantities, notionals, rates, and signed deltas across trades", func() {
			var cumBuyQty, cumSellQty float64
			var cumBuyNotional, cumSellNotional float64
			var buyCount, sellCount float64

			for step := range 10 {
				prior := arena.NewMeasurement("ingress")
				prior.Label = "BTC/USD"
				prior.SeqIdx = int64(step + 1)
				prior.At = now.Add(time.Duration(step) * 100 * time.Millisecond)
				prior.From = prior.At

				price := 50000.0 + float64(step)*10.0
				qty := 1.5 + float64(step)*0.1
				notional := price * qty

				side := "buy"
				if step%2 != 0 {
					side = "sell"
					sellCount++
					cumSellQty += qty
					cumSellNotional += notional
				} else {
					buyCount++
					cumBuyQty += qty
					cumBuyNotional += notional
				}

				prior.SetMetric("price", data.NewMetric[float64](
					"price",
					data.UnitPrice,
					data.TimescaleInstantaneous,
					price,
					10.0,
				).Write(price))
				prior.SetMetric("qty", data.NewMetric[float64](
					"qty",
					data.UnitQuantity,
					data.TimescaleInstantaneous,
					0.0,
					qty,
				).Write(qty))
				prior.SetMetric("best_bid", data.NewMetric[float64](
					"best_bid",
					data.UnitPrice,
					data.TimescaleInstantaneous,
					price,
					10.0,
				).Write(price-5.0))
				prior.SetMetric("best_ask", data.NewMetric[float64](
					"best_ask",
					data.UnitPrice,
					data.TimescaleInstantaneous,
					price,
					10.0,
				).Write(price+5.0))
				prior.SetProvenance("side", side)
				prior.SetProvenance("channel", "trade")

				res := instrument.Step(prior)
				So(res, ShouldNotBeNil)
				So(res.Err, ShouldBeNil)

				totalCount := float64(step + 1)
				expectedGrossQty := cumBuyQty + cumSellQty
				expectedNetQty := cumBuyQty - cumSellQty
				expectedGrossNotional := cumBuyNotional + cumSellNotional
				expectedNetNotional := cumBuyNotional - cumSellNotional

				// Trade counts
				So(res.GetMetric("trade_count").Raw, ShouldEqual, totalCount)
				So(res.GetMetric("trade_count:buy").Raw, ShouldEqual, buyCount)
				So(res.GetMetric("trade_count:sell").Raw, ShouldEqual, sellCount)

				// Executed quantities & volume delta
				So(res.GetMetric("executed_quantity:buy").Raw, ShouldAlmostEqual, cumBuyQty, 1e-9)
				So(res.GetMetric("executed_quantity:sell").Raw, ShouldAlmostEqual, cumSellQty, 1e-9)
				So(res.GetMetric("gross_executed_quantity").Raw, ShouldAlmostEqual, expectedGrossQty, 1e-9)
				So(res.GetMetric("net_executed_quantity").Raw, ShouldAlmostEqual, expectedNetQty, 1e-9)
				So(res.GetMetric("cumulative_volume_delta").Raw, ShouldAlmostEqual, expectedNetQty, 1e-9)

				// Notionals & delta
				So(res.GetMetric("aggressive_notional:buy").Raw, ShouldAlmostEqual, cumBuyNotional, 1e-6)
				So(res.GetMetric("aggressive_notional:sell").Raw, ShouldAlmostEqual, cumSellNotional, 1e-6)
				So(res.GetMetric("gross_notional").Raw, ShouldAlmostEqual, expectedGrossNotional, 1e-6)
				So(res.GetMetric("net_notional").Raw, ShouldAlmostEqual, expectedNetNotional, 1e-6)
				So(res.GetMetric("cumulative_notional_delta").Raw, ShouldAlmostEqual, expectedNetNotional, 1e-6)

				// Fractions
				expectedSignedCountFrac := (buyCount - sellCount) / totalCount
				So(res.GetMetric("signed_count_fraction").Raw, ShouldAlmostEqual, expectedSignedCountFrac, 1e-9)

				expectedSignedNetFrac := expectedNetNotional / expectedGrossNotional
				So(res.GetMetric("signed_net_fraction").Raw, ShouldAlmostEqual, expectedSignedNetFrac, 1e-9)

				// Rates over elapsed span
				span := prior.At.Sub(origin).Seconds()
				if span > 0 {
					So(res.GetMetric("trade_rate").Raw, ShouldAlmostEqual, totalCount/span, 1e-6)
					So(res.GetMetric("gross_notional_rate").Raw, ShouldAlmostEqual, expectedGrossNotional/span, 1e-6)
					So(res.GetMetric("net_notional_rate").Raw, ShouldAlmostEqual, expectedNetNotional/span, 1e-6)
					So(res.GetMetric("buy_notional_rate").Raw, ShouldAlmostEqual, cumBuyNotional/span, 1e-6)
					So(res.GetMetric("sell_notional_rate").Raw, ShouldAlmostEqual, cumSellNotional/span, 1e-6)
				}

				// Standardized price check
				priceMetric := res.GetMetric("price")
				So(priceMetric.Center, ShouldEqual, price)
				So(priceMetric.Scale, ShouldEqual, 10.0)
				So(priceMetric.Standardized, ShouldNotBeNil)
				So(*priceMetric.Standardized, ShouldEqual, 0.0)

				if step > 2 {
					So(res.Maturity, ShouldBeGreaterThan, 0)
					So(res.SNRDefined, ShouldBeTrue)
				}
			}
		})

		Convey("Rejects non-positive price or quantity with validation error", func() {
			prior := arena.NewMeasurement("ingress")
			prior.Label = "BTC/USD"
			prior.SeqIdx = 99
			prior.At = now
			prior.From = now
			prior.SetProvenance("side", "buy")
			prior.SetMetric("price", data.NewMetric[float64](
				"price",
				data.UnitPrice,
				data.TimescaleInstantaneous,
				0.0,
				1.0,
			).Write(0.0)) // Invalid zero price
			prior.SetMetric("qty", data.NewMetric[float64](
				"qty",
				data.UnitQuantity,
				data.TimescaleInstantaneous,
				0.0,
				1.0,
			).Write(1.0))

			res := instrument.Step(prior)
			So(res, ShouldNotBeNil)
			So(res.Err, ShouldNotBeNil)
		})
	})
}
