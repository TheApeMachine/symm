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

func TestPumpDumpSignal(t *testing.T) {
	Convey("Pumpdump / Volume-Clocked Activity instrument publishes complete honest metrics", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner(4096)
		now := time.Now()

		normalizer := spot.NewNormalizer()
		books := broker.NewBook(ctx, normalizer)

		instrument := pumpdump.NewSignal(ctx, arena, books)
		instrument.Transition(nmruntime.READY)

		Convey("Emits unified book touch and trade volume clock metrics per tick", func() {
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

				tradePrice := 50001.0 + float64(step)*0.1
				tradeQty := 1.0 + float64(step)*0.05
				prior.SetMetric("price", data.NewMetric[float64](
					"price",
					data.UnitPrice,
					data.TimescaleInstantaneous,
					tradePrice,
					2.0,
				).Write(tradePrice))
				prior.SetMetric("qty", data.NewMetric[float64](
					"qty",
					data.UnitQuantity,
					data.TimescaleInstantaneous,
					0.0,
					tradeQty,
				).Write(tradeQty))
				prior.SetProvenance("channel", "trade")
				prior.SetProvenance("side", "buy")

				res := instrument.Step(prior)
				So(res, ShouldNotBeNil)

				// Book touch metrics
				_, hasBid := res.LookupMetric("best_bid")
				So(hasBid, ShouldBeTrue)

				_, hasMid := res.LookupMetric("midpoint")
				So(hasMid, ShouldBeTrue)

				_, hasSpread := res.LookupMetric("spread")
				So(hasSpread, ShouldBeTrue)

				_, hasRelSpread := res.LookupMetric("relative_spread")
				So(hasRelSpread, ShouldBeTrue)

				// Trade dynamics
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
