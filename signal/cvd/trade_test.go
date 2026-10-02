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

func TestCVDTradeMetrics(t *testing.T) {
	Convey("CVD trade instrument publishes complete honest metrics", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner(4096)
		instrument := cvd.NewTrade(ctx, arena)
		instrument.Transition(nmruntime.READY)

		now := time.Now()

		for step := 0; step < 10; step++ {
			prior := arena.NewMeasurement("ingress")
			prior.Label = "BTC/USD"
			prior.SeqIdx = int64(step + 1)
			prior.At = now.Add(time.Duration(step) * 100 * time.Millisecond)
			prior.From = prior.At

			price := 50000.0 + float64(step)*10.0
			qty := 1.5
			side := "buy"
			if step%2 != 0 {
				side = "sell"
			}

			prior.WriteMetric("price", price)
			prior.WriteMetric("qty", qty)
			prior.WriteMetric("best_bid", price-5.0)
			prior.WriteMetric("best_ask", price+5.0)
			prior.SetProvenance("side", side)

			res := instrument.Step(prior)
			So(res, ShouldNotBeNil)

			_, hasGross := res.LookupMetric("gross_notional")
			So(hasGross, ShouldBeTrue)

			_, hasNet := res.LookupMetric("net_notional")
			So(hasNet, ShouldBeTrue)

			_, hasDelta := res.LookupMetric("cumulative_volume_delta")
			So(hasDelta, ShouldBeTrue)

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
}
