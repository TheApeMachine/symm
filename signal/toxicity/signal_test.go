package toxicity_test

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
	"github.com/theapemachine/symm/signal/toxicity"
)

func TestToxicitySignal(t *testing.T) {
	Convey("Toxicity instrument measures touch dispositions and trade matching", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner(4096)
		now := time.Now()

		normalizer := spot.NewNormalizer()
		books := broker.NewBook(ctx, normalizer)

		instrument := toxicity.NewSignal(ctx, arena, books)
		instrument.Transition(nmruntime.READY)

		Convey("Publishes unified touch dispositions and trade fill metrics per tick", func() {
			for step := range 10 {
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
									OrderQty:   decimal.NewFromFloat64(10.0 + float64(step%3)),
									Timestamp:  now.Add(time.Duration(step) * 100 * time.Millisecond),
									Event:      "add",
								},
							},
							Asks: []kraken.Level3Order{
								{
									OrderID:    "ask-1",
									LimitPrice: decimal.NewFromFloat64(50002.0),
									OrderQty:   decimal.NewFromFloat64(10.0 - float64(step%2)),
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

				prior.SetMetric("price", data.NewMetric[float64](
					"price",
					data.UnitPrice,
					data.TimescaleInstantaneous,
					50002.0,
					2.0,
				).Write(50002.0))
				prior.SetMetric("qty", data.NewMetric[float64](
					"qty",
					data.UnitQuantity,
					data.TimescaleInstantaneous,
					0.0,
					1.5,
				).Write(1.5))
				prior.SetProvenance("side", "buy")
				prior.SetProvenance("channel", "trade")

				res := instrument.Step(prior)
				So(res, ShouldNotBeNil)

				_, hasBidPrice := res.LookupMetric("best_price:bid")
				So(hasBidPrice, ShouldBeTrue)

				_, hasAskPrice := res.LookupMetric("best_price:ask")
				So(hasAskPrice, ShouldBeTrue)

				_, hasBracket := res.LookupMetric("bracket_trade_quantity")
				So(hasBracket, ShouldBeTrue)

				_, hasFillAsk := res.LookupMetric("touch_fill_quantity:ask")
				So(hasFillAsk, ShouldBeTrue)

				if step > 0 {
					_, hasPrevBid := res.LookupMetric("previous_best_price:bid")
					So(hasPrevBid, ShouldBeTrue)
				}

				if step > 2 {
					So(res.Maturity, ShouldBeGreaterThan, 0)
				}
			}
		})
	})
}
