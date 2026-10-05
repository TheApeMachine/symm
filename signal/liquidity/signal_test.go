package liquidity_test

import (
	"context"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/liquidity"
)

func TestLiquidityTickerMetrics(t *testing.T) {
	Convey("Liquidity ticker instrument validates touch geometry, notional depth, and book imbalances", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner(4096)
		instrument := liquidity.NewSignal(ctx, arena)
		instrument.Transition(nmruntime.READY)

		now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		Convey("Computes exact touch notionals, midpoint, relative spread, and order book imbalance", func() {
			for step := range 10 {
				prior := arena.NewMeasurement("ingress")
				prior.Label = "ETH/USD"
				prior.SeqIdx = int64(step + 1)
				prior.At = now.Add(time.Duration(step) * 100 * time.Millisecond)
				prior.From = prior.At

				bid := 3000.0 + float64(step)*2.0
				ask := bid + 4.0
				bidQty := 5.0 + float64(step)*0.5
				askQty := 3.0 + float64(step)*0.2

				midpoint := (bid + ask) / 2.0
				spread := ask - bid
				relativeSpread := spread / midpoint
				expectedBidNotional := bid * bidQty
				expectedAskNotional := ask * askQty
				expectedTwoSidedNotional := math.Min(expectedBidNotional, expectedAskNotional)
				expectedTotalNotional := expectedBidNotional + expectedAskNotional
				expectedImbalance := (expectedBidNotional - expectedAskNotional) / expectedTotalNotional

				prior.SetMetric("bid", data.NewMetric(
					"bid",
					data.UnitPrice,
					data.TimescaleInstantaneous,
					midpoint,
					spread,
				).Write(bid))
				prior.SetMetric("ask", data.NewMetric(
					"ask",
					data.UnitPrice,
					data.TimescaleInstantaneous,
					midpoint,
					spread,
				).Write(ask))
				prior.SetMetric("bid_qty", data.NewMetric(
					"bid_qty",
					data.UnitQuantity,
					data.TimescaleInstantaneous,
					0.0,
					bidQty,
				).Write(bidQty))
				prior.SetMetric("ask_qty", data.NewMetric(
					"ask_qty",
					data.UnitQuantity,
					data.TimescaleInstantaneous,
					0.0,
					askQty,
				).Write(askQty))

				res := instrument.Step(prior)
				So(res, ShouldNotBeNil)
				So(res.Err, ShouldBeNil)

				// Mathematical touch geometry
				So(res.GetMetric("midpoint").Raw, ShouldAlmostEqual, midpoint, 1e-9)
				So(res.GetMetric("spread").Raw, ShouldAlmostEqual, spread, 1e-9)
				So(res.GetMetric("relative_spread").Raw, ShouldAlmostEqual, relativeSpread, 1e-9)

				// Notional calculations
				So(res.GetMetric("touch_notional:bid").Raw, ShouldAlmostEqual, expectedBidNotional, 1e-6)
				So(res.GetMetric("touch_notional:ask").Raw, ShouldAlmostEqual, expectedAskNotional, 1e-6)
				So(res.GetMetric("two_sided_touch_notional").Raw, ShouldAlmostEqual, expectedTwoSidedNotional, 1e-6)
				So(res.GetMetric("touch_notional_imbalance").Raw, ShouldAlmostEqual, expectedImbalance, 1e-9)

				// Price standardization against midpoint and spread
				bidMetric := res.GetMetric("best_bid_price")
				So(bidMetric.Center, ShouldEqual, midpoint)
				So(bidMetric.Scale, ShouldEqual, spread)
				So(bidMetric.Standardized, ShouldNotBeNil)
				So(*bidMetric.Standardized, ShouldAlmostEqual, -0.5, 1e-9)

				askMetric := res.GetMetric("best_ask_price")
				So(askMetric.Center, ShouldEqual, midpoint)
				So(askMetric.Scale, ShouldEqual, spread)
				So(askMetric.Standardized, ShouldNotBeNil)
				So(*askMetric.Standardized, ShouldAlmostEqual, 0.5, 1e-9)

				if step > 2 {
					So(res.Maturity, ShouldBeGreaterThan, 0)
					So(res.SNRDefined, ShouldBeTrue)
				}
			}
		})

		Convey("Rejects crossed quote (bid >= ask) with validation error", func() {
			prior := arena.NewMeasurement("ingress")
			prior.Label = "ETH/USD"
			prior.SeqIdx = 100
			prior.At = now
			prior.From = now
			prior.SetMetric("bid", data.NewMetric(
				"bid",
				data.UnitPrice,
				data.TimescaleInstantaneous,
				3005.0,
				1.0,
			).Write(3005.0))
			prior.SetMetric("ask", data.NewMetric(
				"ask",
				data.UnitPrice,
				data.TimescaleInstantaneous,
				3000.0, // crossed: ask < bid
				1.0,
			).Write(3000.0))
			prior.SetMetric("bid_qty", data.NewMetric(
				"bid_qty",
				data.UnitQuantity,
				data.TimescaleInstantaneous,
				0.0,
				1.0,
			).Write(1.0))
			prior.SetMetric("ask_qty", data.NewMetric(
				"ask_qty",
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
