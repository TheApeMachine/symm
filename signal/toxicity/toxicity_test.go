package toxicity_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/toxicity"
)

func TestToxicitySignals(t *testing.T) {
	Convey("Toxicity instruments measure touch dispositions and trade matching", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner(4096)
		now := time.Now()

		Convey("Level3 instrument publishes retreat, withdrawal, and replenishment metrics", func() {
			instrument := toxicity.NewLevel3(ctx, arena, nil)
			instrument.Transition(nmruntime.READY)

			for step := 0; step < 10; step++ {
				prior := arena.NewMeasurement("ingress")
				prior.Label = "BTC/USD"
				prior.SeqIdx = int64(step + 1)
				prior.At = now.Add(time.Duration(step) * 100 * time.Millisecond)
				prior.From = prior.At

				prior.WriteMetric("best_bid", 50000.0)
				prior.WriteMetric("best_ask", 50002.0)
				prior.WriteMetric("touch_quantity:bid", 10.0+float64(step%3))
				prior.WriteMetric("touch_quantity:ask", 10.0-float64(step%2))

				res := instrument.Step(prior)
				So(res, ShouldNotBeNil)

				_, hasBidPrice := res.LookupMetric("best_price:bid")
				So(hasBidPrice, ShouldBeTrue)

				_, hasAskPrice := res.LookupMetric("best_price:ask")
				So(hasAskPrice, ShouldBeTrue)

				if step > 0 {
					_, hasPrevBid := res.LookupMetric("previous_best_price:bid")
					So(hasPrevBid, ShouldBeTrue)
				}

				if step > 2 {
					So(res.Maturity, ShouldBeGreaterThan, 0)
				}
			}
		})

		Convey("Trade instrument attributes fills against touch quotes", func() {
			instrument := toxicity.NewTrade(ctx, arena)
			instrument.Transition(nmruntime.READY)

			for step := 0; step < 10; step++ {
				prior := arena.NewMeasurement("ingress")
				prior.Label = "BTC/USD"
				prior.SeqIdx = int64(step + 1)
				prior.At = now.Add(time.Duration(step) * 100 * time.Millisecond)
				prior.From = prior.At

				prior.WriteMetric("price", 50000.0)
				prior.WriteMetric("qty", 1.0)
				prior.WriteMetric("best_bid", 50000.0)
				prior.WriteMetric("best_ask", 50002.0)
				prior.WriteMetric("touch_quantity:bid", 10.0)
				prior.WriteMetric("touch_quantity:ask", 10.0)
				prior.SetProvenance("side", "sell")

				res := instrument.Step(prior)
				So(res, ShouldNotBeNil)

				_, hasBracket := res.LookupMetric("bracket_trade_quantity")
				So(hasBracket, ShouldBeTrue)

				_, hasMatchedBid := res.LookupMetric("matched_touch_trade_quantity:bid")
				So(hasMatchedBid, ShouldBeTrue)

				_, hasFillBid := res.LookupMetric("touch_fill_quantity:bid")
				So(hasFillBid, ShouldBeTrue)

				if step > 2 {
					So(res.Maturity, ShouldBeGreaterThan, 0)
				}
			}
		})
	})
}
