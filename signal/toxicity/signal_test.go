package toxicity_test

import (
	"context"
	"math"
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
	Convey("Toxicity instrument calculates exact touch dispositions and trade fill matching", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner(4096)
		now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		normalizer := spot.NewNormalizer()
		books := broker.NewBook(ctx, normalizer)

		instrument := toxicity.NewSignal(ctx, arena, books)
		instrument.Transition(nmruntime.READY)

		Convey("Trade matching at the touch calculates exact fill quantity, fraction, and fill rate", func() {
			bidPrice := 50000.0
			askPrice := 50002.0
			touchQty := 10.0

			books.Update(&kraken.Level3{
				Channel: "level3",
				Type:    "snapshot",
				Data: []kraken.Level3Data{
					{
						Symbol: "BTC/USD",
						Bids: []kraken.Level3Order{
							{
								OrderID:    "bid-1",
								LimitPrice: decimal.NewFromFloat64(bidPrice),
								OrderQty:   decimal.NewFromFloat64(touchQty),
								Timestamp:  now,
								Event:      "add",
							},
						},
						Asks: []kraken.Level3Order{
							{
								OrderID:    "ask-1",
								LimitPrice: decimal.NewFromFloat64(askPrice),
								OrderQty:   decimal.NewFromFloat64(touchQty),
								Timestamp:  now,
								Event:      "add",
							},
						},
					},
				},
			})

			// Trade 1: aggressive buy of 1.5 at the ask (50002.0)
			trade1Qty := 1.5
			prior1 := arena.NewMeasurement("ingress")
			prior1.Label = "BTC/USD"
			prior1.SeqIdx = 1
			prior1.At = now
			prior1.From = now
			prior1.SetMetric("price", data.NewMetric[float64]("price", data.UnitPrice, data.TimescaleInstantaneous, askPrice, 1.0).Write(askPrice))
			prior1.SetMetric("qty", data.NewMetric[float64]("qty", data.UnitQuantity, data.TimescaleInstantaneous, 0.0, trade1Qty).Write(trade1Qty))
			prior1.SetProvenance("side", "buy")
			prior1.SetProvenance("channel", "trade")

			res1 := instrument.Step(prior1)
			So(res1, ShouldNotBeNil)
			So(res1.Err, ShouldBeNil)

			// Touch geometry
			So(res1.GetMetric("best_price:bid").Raw, ShouldEqual, bidPrice)
			So(res1.GetMetric("best_price:ask").Raw, ShouldEqual, askPrice)
			So(res1.GetMetric("touch_quantity:bid").Raw, ShouldEqual, touchQty)
			So(res1.GetMetric("touch_quantity:ask").Raw, ShouldEqual, touchQty)

			// Trade matching against ask touch
			So(res1.GetMetric("bracket_trade_quantity").Raw, ShouldAlmostEqual, trade1Qty, 1e-9)
			So(res1.GetMetric("matched_touch_trade_quantity:ask").Raw, ShouldAlmostEqual, trade1Qty, 1e-9)
			So(res1.GetMetric("touch_fill_quantity:ask").Raw, ShouldAlmostEqual, trade1Qty, 1e-9)
			So(res1.GetMetric("touch_fill_fraction:ask").Raw, ShouldAlmostEqual, trade1Qty/touchQty, 1e-9)

			// Opposite side (bid) untouched
			So(res1.GetMetric("matched_touch_trade_quantity:bid").Raw, ShouldAlmostEqual, 0.0, 1e-9)
			So(res1.GetMetric("touch_fill_quantity:bid").Raw, ShouldAlmostEqual, 0.0, 1e-9)
			So(res1.GetMetric("touch_fill_fraction:bid").Raw, ShouldAlmostEqual, 0.0, 1e-9)

			// Trade 2: 100ms later, aggressive buy of 2.5 at ask
			trade2Qty := 2.5
			trade2At := now.Add(100 * time.Millisecond)
			prior2 := arena.NewMeasurement("ingress")
			prior2.Label = "BTC/USD"
			prior2.SeqIdx = 2
			prior2.At = trade2At
			prior2.From = trade2At
			prior2.SetMetric("price", data.NewMetric[float64]("price", data.UnitPrice, data.TimescaleInstantaneous, askPrice, 1.0).Write(askPrice))
			prior2.SetMetric("qty", data.NewMetric[float64]("qty", data.UnitQuantity, data.TimescaleInstantaneous, 0.0, trade2Qty).Write(trade2Qty))
			prior2.SetProvenance("side", "buy")
			prior2.SetProvenance("channel", "trade")

			res2 := instrument.Step(prior2)
			So(res2, ShouldNotBeNil)
			So(res2.Err, ShouldBeNil)

			expectedCumQty := trade1Qty + trade2Qty
			So(res2.GetMetric("touch_fill_quantity:ask").Raw, ShouldAlmostEqual, expectedCumQty, 1e-9)
			So(res2.GetMetric("touch_fill_fraction:ask").Raw, ShouldAlmostEqual, expectedCumQty/touchQty, 1e-9)
			So(res2.GetMetric("touch_fill_rate:ask").Raw, ShouldAlmostEqual, expectedCumQty/0.1, 1e-6)
		})

		Convey("Touch disposition detects adverse price retreat and liquidity withdrawal", func() {
			dispositionInstrument := toxicity.NewSignal(ctx, arena, books)
			dispositionInstrument.Transition(nmruntime.READY)

			// Initial touch book
			books.Update(&kraken.Level3{
				Channel: "level3",
				Type:    "snapshot",
				Data: []kraken.Level3Data{
					{
						Symbol: "BTC/USD",
						Bids: []kraken.Level3Order{
							{
								OrderID:    "bid-disp-1",
								LimitPrice: decimal.NewFromFloat64(50000.0),
								OrderQty:   decimal.NewFromFloat64(10.0),
								Timestamp:  now,
								Event:      "add",
							},
						},
						Asks: []kraken.Level3Order{
							{
								OrderID:    "ask-disp-1",
								LimitPrice: decimal.NewFromFloat64(50002.0),
								OrderQty:   decimal.NewFromFloat64(10.0),
								Timestamp:  now,
								Event:      "add",
							},
						},
					},
				},
			})

			prior1 := arena.NewMeasurement("ingress")
			prior1.Label = "BTC/USD"
			prior1.SeqIdx = 10
			prior1.At = now
			prior1.From = now
			prior1.SetMetric("price", data.NewMetric[float64]("price", data.UnitPrice, data.TimescaleInstantaneous, 50001.0, 1.0).Write(50001.0))
			prior1.SetMetric("qty", data.NewMetric[float64]("qty", data.UnitQuantity, data.TimescaleInstantaneous, 0.0, 1.0).Write(1.0))
			prior1.SetProvenance("side", "buy")
			prior1.SetProvenance("channel", "trade")

			dispositionInstrument.Step(prior1)

			// Step 2: Bid retreats down to 49990 (50000 was canceled)
			step2At := now.Add(200 * time.Millisecond)
			books.Update(&kraken.Level3{
				Channel: "level3",
				Type:    "snapshot",
				Data: []kraken.Level3Data{
					{
						Symbol: "BTC/USD",
						Bids: []kraken.Level3Order{
							{
								OrderID:    "bid-disp-2",
								LimitPrice: decimal.NewFromFloat64(49990.0), // Retreat by 10 points
								OrderQty:   decimal.NewFromFloat64(8.0),
								Timestamp:  step2At,
								Event:      "add",
							},
						},
						Asks: []kraken.Level3Order{
							{
								OrderID:    "ask-disp-1",
								LimitPrice: decimal.NewFromFloat64(50002.0),
								OrderQty:   decimal.NewFromFloat64(10.0),
								Timestamp:  step2At,
								Event:      "add",
							},
						},
					},
				},
			})

			prior2 := arena.NewMeasurement("ingress")
			prior2.Label = "BTC/USD"
			prior2.SeqIdx = 11
			prior2.At = step2At
			prior2.From = step2At
			prior2.SetMetric("price", data.NewMetric[float64]("price", data.UnitPrice, data.TimescaleInstantaneous, 49995.0, 1.0).Write(49995.0))
			prior2.SetMetric("qty", data.NewMetric[float64]("qty", data.UnitQuantity, data.TimescaleInstantaneous, 0.0, 1.0).Write(1.0))
			prior2.SetProvenance("side", "sell")
			prior2.SetProvenance("channel", "trade")

			res2 := dispositionInstrument.Step(prior2)
			So(res2, ShouldNotBeNil)

			So(res2.GetMetric("previous_best_price:bid").Raw, ShouldEqual, 50000.0)
			So(res2.GetMetric("best_price:bid").Raw, ShouldEqual, 49990.0)
			So(res2.GetMetric("previous_touch_quantity:bid").Raw, ShouldEqual, 10.0)
			So(res2.GetMetric("touch_quantity:bid").Raw, ShouldEqual, 8.0)

			// Log change on retreating bid is negative: ln(49990 / 50000) < 0
			logChange := res2.GetMetric("touch_price_log_change:bid").Raw
			expectedLogChange := math.Log(49990.0 / 50000.0)
			So(logChange, ShouldAlmostEqual, expectedLogChange, 1e-6)

			// Retreated quantity must equal previous touch quantity (10.0)
			So(res2.GetMetric("retreated_quantity:bid").Raw, ShouldAlmostEqual, 10.0, 1e-9)
			So(res2.GetMetric("retreat_rate:bid").Raw, ShouldAlmostEqual, 10.0/0.2, 1e-6)
		})

		Convey("Rejects non-positive price or quantity with validation error", func() {
			prior := arena.NewMeasurement("ingress")
			prior.Label = "BTC/USD"
			prior.SeqIdx = 99
			prior.At = now
			prior.From = now
			prior.SetProvenance("side", "buy")
			prior.SetProvenance("channel", "trade")
			prior.SetMetric("price", data.NewMetric[float64]("price", data.UnitPrice, data.TimescaleInstantaneous, 0.0, 1.0).Write(0.0)) // Invalid zero price
			prior.SetMetric("qty", data.NewMetric[float64]("qty", data.UnitQuantity, data.TimescaleInstantaneous, 0.0, 1.0).Write(1.0))

			res := instrument.Step(prior)
			So(res, ShouldNotBeNil)
			So(res.Err, ShouldNotBeNil)
		})
	})
}
