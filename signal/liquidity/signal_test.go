package liquidity_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/liquidity"
)

func TestLiquidityTickerMetrics(t *testing.T) {
	Convey("Liquidity ticker instrument publishes complete honest metrics", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner(4096)
		instrument := liquidity.NewSignal(ctx, arena)
		instrument.Transition(nmruntime.READY)

		now := time.Now()

		for step := 0; step < 10; step++ {
			prior := arena.NewMeasurement("ingress")
			prior.Label = "ETH/USD"
			prior.SeqIdx = int64(step + 1)
			prior.At = now.Add(time.Duration(step) * 100 * time.Millisecond)
			prior.From = prior.At

			bid := 3000.0 + float64(step)
			ask := bid + 2.0
			bidQty := 5.0
			askQty := 4.0

			midpoint := (bid + ask) / 2.0
			spread := ask - bid
			totalQty := bidQty + askQty

			prior.SetMetric("bid", data.NewMetric[float64](
				"bid",
				data.UnitPrice,
				data.TimescaleInstantaneous,
				midpoint,
				spread,
			).Write(bid))
			prior.SetMetric("ask", data.NewMetric[float64](
				"ask",
				data.UnitPrice,
				data.TimescaleInstantaneous,
				midpoint,
				spread,
			).Write(ask))
			prior.SetMetric("bid_qty", data.NewMetric[float64](
				"bid_qty",
				data.UnitQuantity,
				data.TimescaleInstantaneous,
				0.0,
				totalQty,
			).Write(bidQty))
			prior.SetMetric("ask_qty", data.NewMetric[float64](
				"ask_qty",
				data.UnitQuantity,
				data.TimescaleInstantaneous,
				0.0,
				totalQty,
			).Write(askQty))

			res := instrument.Step(prior)
			So(res, ShouldNotBeNil)

			_, hasMid := res.LookupMetric("midpoint")
			So(hasMid, ShouldBeTrue)

			_, hasSpread := res.LookupMetric("spread")
			So(hasSpread, ShouldBeTrue)

			_, hasRel := res.LookupMetric("relative_spread")
			So(hasRel, ShouldBeTrue)

			_, hasImb := res.LookupMetric("touch_notional_imbalance")
			So(hasImb, ShouldBeTrue)

			// Assert price standardization
			bidMetric := res.GetMetric("best_bid_price")
			So(bidMetric.Center, ShouldEqual, (bid+ask)/2.0)
			So(bidMetric.Scale, ShouldEqual, 2.0)
			So(bidMetric.Standardized, ShouldNotBeNil)
			So(*bidMetric.Standardized, ShouldEqual, -0.5)

			askMetric := res.GetMetric("best_ask_price")
			So(askMetric.Center, ShouldEqual, (bid+ask)/2.0)
			So(askMetric.Scale, ShouldEqual, 2.0)
			So(askMetric.Standardized, ShouldNotBeNil)
			So(*askMetric.Standardized, ShouldEqual, 0.5)

			if step > 2 {
				So(res.Maturity, ShouldBeGreaterThan, 0)
				So(res.SNRDefined, ShouldBeTrue)
			}
		}
	})
}
