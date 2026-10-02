package pumpdump_test

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/pumpdump"
)

func TestPumpDumpSignals(t *testing.T) {
	Convey("Pumpdump / Volume-Clocked Activity instruments publish complete honest metrics", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner(4096)
		now := time.Now()

		Convey("Ticker instrument emits touch geometry and spread dynamics", func() {
			instrument := pumpdump.NewTicker(ctx, arena)
			instrument.Transition(nmruntime.READY)

			for step := 0; step < 10; step++ {
				prior := arena.NewMeasurement("ingress")
				prior.Label = "BTC/USD"
				prior.SeqIdx = int64(step + 1)
				prior.At = now.Add(time.Duration(step) * 100 * time.Millisecond)
				prior.From = prior.At

				prior.WriteMetric("best_bid", 50000.0)
				prior.WriteMetric("best_ask", 50002.0+float64(step)*0.1)

				res := instrument.Step(prior)
				So(res, ShouldNotBeNil)

				_, hasBid := res.LookupMetric("best_bid")
				So(hasBid, ShouldBeTrue)

				_, hasMid := res.LookupMetric("midpoint")
				So(hasMid, ShouldBeTrue)

				_, hasSpread := res.LookupMetric("spread")
				So(hasSpread, ShouldBeTrue)

				_, hasRelSpread := res.LookupMetric("relative_spread")
				So(hasRelSpread, ShouldBeTrue)

				if step > 2 {
					So(res.Maturity, ShouldBeGreaterThan, 0)
				}
			}
		})

		Convey("Level3 instrument extracts book touch and computes spread dynamics", func() {
			normalizer := spot.NewNormalizer()
			books := broker.NewBook(ctx, normalizer)

			instrument := pumpdump.NewLevel3(ctx, arena, books)
			instrument.Transition(nmruntime.READY)

			for step := 0; step < 10; step++ {
				books.Update(&kraken.Level3{
					Channel: "level3",
					Type:    "snapshot",
					Data: []kraken.Level3Data{
						{
							Symbol: "BTC/USD",
							Bids: []kraken.Level3Order{
								{
									OrderID:    "bid-1",
									LimitPrice: decimal.NewFromFloat64(50000.0),
									OrderQty:   decimal.NewFromFloat64(1.0),
									Timestamp:  now.Add(time.Duration(step) * 100 * time.Millisecond),
									Event:      "add",
								},
							},
							Asks: []kraken.Level3Order{
								{
									OrderID:    "ask-1",
									LimitPrice: decimal.NewFromFloat64(50002.0 + float64(step)*0.1),
									OrderQty:   decimal.NewFromFloat64(1.0),
									Timestamp:  now.Add(time.Duration(step) * 100 * time.Millisecond),
									Event:      "add",
								},
							},
						},
					},
				})

				prior := arena.NewMeasurement("ingress")
				prior.Label = "BTC/USD"
				prior.SeqIdx = int64(step + 1)
				prior.At = now.Add(time.Duration(step) * 100 * time.Millisecond)
				prior.From = prior.At

				res := instrument.Step(prior)
				So(res, ShouldNotBeNil)

				_, hasMid := res.LookupMetric("midpoint")
				So(hasMid, ShouldBeTrue)

				_, hasSpread := res.LookupMetric("spread")
				So(hasSpread, ShouldBeTrue)

				if step > 2 {
					So(res.Maturity, ShouldBeGreaterThan, 0)
				}
			}
		})

		Convey("Trade instrument clocks volume bars and calculates rates and midpoint response", func() {
			instrument := pumpdump.NewTrade(ctx, arena)
			instrument.Transition(nmruntime.READY)

			for step := 0; step < 10; step++ {
				prior := arena.NewMeasurement("ingress")
				prior.Label = "BTC/USD"
				prior.SeqIdx = int64(step + 1)
				prior.At = now.Add(time.Duration(step) * 100 * time.Millisecond)
				prior.From = prior.At

				prior.WriteMetric("price", 50000.0+float64(step)*2.0)
				prior.WriteMetric("qty", 1.0+float64(step)*0.1)
				prior.WriteMetric("best_bid", 49999.0+float64(step)*2.0)
				prior.WriteMetric("best_ask", 50001.0+float64(step)*2.0)

				res := instrument.Step(prior)
				So(res, ShouldNotBeNil)

				_, hasTradePrice := res.LookupMetric("trade_price")
				So(hasTradePrice, ShouldBeTrue)

				_, hasTradeQty := res.LookupMetric("trade_quantity")
				So(hasTradeQty, ShouldBeTrue)

				_, hasTradeNotional := res.LookupMetric("trade_notional")
				So(hasTradeNotional, ShouldBeTrue)

				if step > 2 {
					So(res.Maturity, ShouldBeGreaterThan, 0)
				}
			}
		})
	})
}
