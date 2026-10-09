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

func trade(at time.Time, seq int64, side string, price, qty float64) *data.Measurement {
	prior := data.NewMeasurement(
		1, "BTC/USD", "spot:trade", seq, seq,
		&data.StringEntry{Key: "type", Value: "trade"},
		&data.StringEntry{Key: "side", Value: side},
	)
	prior.At = at
	prior.From = at

	return prior.Write(
		data.NewMetric("price", price, data.UnitPrice, data.TimescaleInstantaneous),
		data.NewMetric("qty", qty, data.UnitQuantity, data.TimescaleInstantaneous),
	)
}

func metric(measurement *data.Measurement, label string) (float64, bool) {
	for entry := range measurement.Read(label) {
		if entry.Err != nil {
			return 0, false
		}

		return entry.Metric.Raw, true
	}

	return 0, false
}

func metricValue(measurement *data.Measurement, label string) float64 {
	value, held := metric(measurement, label)
	So(held, ShouldBeTrue)
	return value
}

func touch(books *broker.Book, at time.Time, bidID string, bid, bidQty float64, askID string, ask, askQty float64) {
	books.Update(&kraken.Level3{
		Channel: "level3",
		Type:    "snapshot",
		Data: []kraken.Level3Data{{
			Symbol: "BTC/USD",
			Bids: []kraken.Level3Order{{
				OrderID: bidID, LimitPrice: decimal.NewFromFloat64(bid), OrderQty: decimal.NewFromFloat64(bidQty),
				Timestamp: at, Event: "add",
			}},
			Asks: []kraken.Level3Order{{
				OrderID: askID, LimitPrice: decimal.NewFromFloat64(ask), OrderQty: decimal.NewFromFloat64(askQty),
				Timestamp: at, Event: "add",
			}},
		}},
	})
}

func TestToxicitySignal(t *testing.T) {
	Convey("Toxicity instrument calculates exact touch dispositions and trade fill matching", t, func() {
		ctx := context.Background()
		now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		books := broker.NewBook(ctx, spot.NewNormalizer())

		instrument := toxicity.NewSignal(ctx, books)
		instrument.Transition(nmruntime.READY)

		Convey("Trade matching at the touch calculates exact fill quantity, fraction, and fill rate", func() {
			bidPrice, askPrice, touchQty := 50000.0, 50002.0, 10.0
			touch(books, now, "bid-1", bidPrice, touchQty, "ask-1", askPrice, touchQty)

			trade1Qty := 1.5
			res1 := instrument.Step(trade(now, 1, "buy", askPrice, trade1Qty))
			So(res1, ShouldNotBeNil)
			So(instrument.Error(), ShouldBeNil)
			So(res1.Source, ShouldEqual, "toxicity")

			So(metricValue(res1, "best_price:bid"), ShouldEqual, bidPrice)
			So(metricValue(res1, "best_price:ask"), ShouldEqual, askPrice)
			So(metricValue(res1, "touch_quantity:bid"), ShouldEqual, touchQty)
			So(metricValue(res1, "touch_quantity:ask"), ShouldEqual, touchQty)

			// The first observation opens the first bracket: nothing is
			// attributed to a touch that was not observed before the trade.
			for _, label := range []string{
				"bracket_trade_quantity", "touch_fill_quantity:ask", "touch_fill_fraction:ask",
				"previous_best_price:bid", "touch_fill_rate:ask", "unfilled_residual_quantity:ask",
			} {
				_, held := metric(res1, label)
				So(held, ShouldBeFalse)
			}

			So(res1.From, ShouldEqual, now)

			trade2Qty := 2.5
			trade2At := now.Add(100 * time.Millisecond)
			res2 := instrument.Step(trade(trade2At, 2, "buy", askPrice, trade2Qty))
			So(res2, ShouldNotBeNil)
			So(instrument.Error(), ShouldBeNil)

			// Bracket (now, now+100ms]: one buy executed the previous ask.
			So(metricValue(res2, "bracket_trade_quantity"), ShouldAlmostEqual, trade2Qty, 1e-9)
			So(metricValue(res2, "matched_touch_trade_quantity:ask"), ShouldAlmostEqual, trade2Qty, 1e-9)
			So(metricValue(res2, "touch_fill_quantity:ask"), ShouldAlmostEqual, trade2Qty, 1e-9)
			So(metricValue(res2, "touch_fill_fraction:ask"), ShouldAlmostEqual, trade2Qty/touchQty, 1e-9)
			So(metricValue(res2, "touch_fill_rate:ask"), ShouldAlmostEqual, trade2Qty/0.1, 1e-6)
			So(metricValue(res2, "unfilled_residual_quantity:ask"), ShouldAlmostEqual, touchQty-trade2Qty, 1e-9)
			So(metricValue(res2, "net_replenished_quantity:ask"), ShouldAlmostEqual, trade2Qty, 1e-9)
			So(metricValue(res2, "touch_fill_quantity:bid"), ShouldAlmostEqual, 0.0, 1e-9)
			So(res2.From, ShouldEqual, now)

			// A trade away from the touch is bracket activity, not a fill.
			res3 := instrument.Step(trade(trade2At.Add(100*time.Millisecond), 3, "buy", askPrice+5, 1.0))
			So(res3, ShouldNotBeNil)
			So(metricValue(res3, "bracket_trade_quantity"), ShouldAlmostEqual, 1.0, 1e-9)
			So(metricValue(res3, "touch_fill_quantity:ask"), ShouldAlmostEqual, 0.0, 1e-9)
			So(metricValue(res3, "touch_fill_fraction:ask"), ShouldAlmostEqual, 0.0, 1e-9)
			So(res3.From, ShouldEqual, trade2At)
		})

		Convey("Trades sharing the previous observation's timestamp accrue to the next bracket", func() {
			touch(books, now, "bid-ts", 50000.0, 10.0, "ask-ts", 50002.0, 10.0)
			So(instrument.Step(trade(now, 30, "buy", 50002.0, 1.0)), ShouldNotBeNil)

			same := instrument.Step(trade(now, 31, "buy", 50002.0, 1.0))
			So(same, ShouldNotBeNil)
			_, held := metric(same, "touch_fill_quantity:ask")
			So(held, ShouldBeFalse)

			res := instrument.Step(trade(now.Add(time.Second), 32, "buy", 50002.0, 2.0))
			So(res, ShouldNotBeNil)
			So(metricValue(res, "bracket_trade_quantity"), ShouldAlmostEqual, 3.0, 1e-9)
			So(metricValue(res, "touch_fill_quantity:ask"), ShouldAlmostEqual, 3.0, 1e-9)
		})

		Convey("A retreat removes only the unfilled residual, and fills are capped by display", func() {
			touch(books, now, "bid-r-1", 50000.0, 10.0, "ask-r-1", 50002.0, 2.0)
			So(instrument.Step(trade(now, 40, "buy", 50001.0, 1.0)), ShouldNotBeNil)

			So(instrument.Step(trade(now, 41, "sell", 50000.0, 4.0)), ShouldNotBeNil)

			later := now.Add(200 * time.Millisecond)
			touch(books, later, "bid-r-2", 49990.0, 8.0, "ask-r-2", 50003.0, 1.0)

			res := instrument.Step(trade(later, 42, "buy", 50002.0, 5.0))
			So(res, ShouldNotBeNil)

			// Bid: E = 4 of Q0 = 10, so R = U = 6. Ask: E* = 5 but only 2
			// were displayed, so E = 2 and nothing retreated.
			So(metricValue(res, "retreated_quantity:bid"), ShouldAlmostEqual, 6.0, 1e-9)
			So(metricValue(res, "retreat_fraction:bid"), ShouldAlmostEqual, 0.6, 1e-9)
			So(metricValue(res, "matched_touch_trade_quantity:ask"), ShouldAlmostEqual, 5.0, 1e-9)
			So(metricValue(res, "touch_fill_quantity:ask"), ShouldAlmostEqual, 2.0, 1e-9)
			So(metricValue(res, "unfilled_residual_quantity:ask"), ShouldAlmostEqual, 0.0, 1e-9)
			So(metricValue(res, "retreat_fraction:ask"), ShouldAlmostEqual, 0.0, 1e-9)
		})

		Convey("Touch disposition detects adverse price retreat", func() {
			touch(books, now, "bid-disp-1", 50000.0, 10.0, "ask-disp-1", 50002.0, 10.0)
			So(instrument.Step(trade(now, 10, "buy", 50001.0, 1.0)), ShouldNotBeNil)

			step2At := now.Add(200 * time.Millisecond)
			touch(books, step2At, "bid-disp-2", 49990.0, 8.0, "ask-disp-1", 50002.0, 10.0)

			res2 := instrument.Step(trade(step2At, 11, "sell", 49995.0, 1.0))
			So(res2, ShouldNotBeNil)
			So(instrument.Error(), ShouldBeNil)

			So(metricValue(res2, "previous_best_price:bid"), ShouldEqual, 50000.0)
			So(metricValue(res2, "best_price:bid"), ShouldEqual, 49990.0)
			So(metricValue(res2, "previous_touch_quantity:bid"), ShouldEqual, 10.0)
			So(metricValue(res2, "touch_quantity:bid"), ShouldEqual, 8.0)

			So(metricValue(res2, "touch_price_log_change:bid"), ShouldAlmostEqual, math.Log(49990.0/50000.0), 1e-12)
			So(metricValue(res2, "touch_price_log_change:ask"), ShouldAlmostEqual, 0.0, 1e-12)

			So(metricValue(res2, "retreat_fraction:bid"), ShouldEqual, 1.0)
			So(metricValue(res2, "retreated_quantity:bid"), ShouldAlmostEqual, 10.0, 1e-9)
			So(metricValue(res2, "retreat_rate:bid"), ShouldAlmostEqual, 10.0/0.2, 1e-6)

			// A retreating bid has no same-price disposition.
			_, held := metric(res2, "net_withdrawn_quantity:bid")
			So(held, ShouldBeFalse)

			// The unchanged ask neither retreats nor withdraws.
			for _, label := range []string{
				"retreat_fraction:ask", "retreated_quantity:ask",
				"net_withdrawn_quantity:ask", "net_replenished_quantity:ask",
			} {
				val, held := metric(res2, label)
				So(held, ShouldBeTrue)
				So(val, ShouldEqual, 0.0)
			}
		})

		Convey("Touch disposition measures withdrawal and replenishment at a held price", func() {
			touch(books, now, "bid-w-1", 50000.0, 10.0, "ask-w-1", 50002.0, 4.0)
			So(instrument.Step(trade(now, 20, "buy", 50001.0, 1.0)), ShouldNotBeNil)

			later := now.Add(500 * time.Millisecond)
			touch(books, later, "bid-w-1", 50000.0, 6.0, "ask-w-1", 50002.0, 7.0)

			res := instrument.Step(trade(later, 21, "buy", 50001.0, 1.0))
			So(res, ShouldNotBeNil)
			So(instrument.Error(), ShouldBeNil)

			So(metricValue(res, "net_withdrawn_quantity:bid"), ShouldAlmostEqual, 4.0, 1e-9)
			So(metricValue(res, "net_withdrawal_fraction:bid"), ShouldAlmostEqual, 0.4, 1e-9)
			So(metricValue(res, "net_withdrawal_rate:bid"), ShouldAlmostEqual, 4.0/0.5, 1e-6)
			So(metricValue(res, "net_replenished_quantity:ask"), ShouldAlmostEqual, 3.0, 1e-9)
			So(metricValue(res, "net_replenishment_fraction:ask"), ShouldAlmostEqual, 0.75, 1e-9)
			So(metricValue(res, "net_replenishment_rate:ask"), ShouldAlmostEqual, 3.0/0.5, 1e-6)

			for _, label := range []string{
				"retreat_fraction:bid", "net_replenished_quantity:bid", "net_withdrawn_quantity:ask",
			} {
				val, held := metric(res, label)
				So(held, ShouldBeTrue)
				So(val, ShouldEqual, 0.0)
			}
		})

		Convey("A non-positive trade price yields no measurement", func() {
			touch(books, now, "bid-1", 50000.0, 10.0, "ask-1", 50002.0, 10.0)
			So(instrument.Step(trade(now, 99, "buy", 0.0, 1.0)), ShouldBeNil)
			So(instrument.Error(), ShouldBeNil)
		})

		Convey("The touch comes only from the book manager, never from the trade frame", func() {
			touch(books, now, "bid-src", 50000.0, 10.0, "ask-src", 50002.0, 4.0)

			prior := data.NewMeasurement(
				1, "BTC/USD", "spot:trade", 120, 120,
				&data.StringEntry{Key: "type", Value: "trade"},
				&data.StringEntry{Key: "side", Value: "buy"},
			)
			prior.At = now
			prior.From = now
			prior = prior.Write(
				data.NewMetric("price", 50002.0, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("qty", 1.0, data.UnitQuantity, data.TimescaleInstantaneous),
				data.NewMetric("bid", 1.0, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("ask", 2.0, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("bid_qty", 99.0, data.UnitQuantity, data.TimescaleInstantaneous),
				data.NewMetric("ask_qty", 99.0, data.UnitQuantity, data.TimescaleInstantaneous),
			)

			res := instrument.Step(prior)
			So(res, ShouldNotBeNil)
			So(instrument.Error(), ShouldBeNil)
			So(metricValue(res, "best_price:bid"), ShouldEqual, 50000.0)
			So(metricValue(res, "best_price:ask"), ShouldEqual, 50002.0)
			So(metricValue(res, "touch_quantity:bid"), ShouldEqual, 10.0)
			So(metricValue(res, "touch_quantity:ask"), ShouldEqual, 4.0)
		})

		Convey("A trade for a symbol without a book yields no measurement and stays healthy", func() {
			prior := data.NewMeasurement(
				1, "SOL/USD", "spot:trade", 130, 130,
				&data.StringEntry{Key: "type", Value: "trade"},
				&data.StringEntry{Key: "side", Value: "buy"},
			)
			prior.At = now
			prior.From = now

			So(instrument.Step(prior.Write(
				data.NewMetric("price", 150.0, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("qty", 1.0, data.UnitQuantity, data.TimescaleInstantaneous),
			)), ShouldBeNil)
			So(instrument.Error(), ShouldBeNil)
		})

		Convey("A crossed touch is dropped with a warning without halting the system", func() {
			touch(books, now, "bid-x", 50010.0, 1.0, "ask-x", 50000.0, 1.0)

			So(instrument.Step(trade(now, 150, "buy", 50005.0, 1.0)), ShouldBeNil)
			So(instrument.Error(), ShouldBeNil)
			So(instrument.Status(), ShouldEqual, nmruntime.READY)
		})

		Convey("A trade frame without qty is an error, not a silent drop", func() {
			touch(books, now, "bid-1", 50000.0, 10.0, "ask-1", 50002.0, 10.0)

			prior := data.NewMeasurement(
				1, "BTC/USD", "spot:trade", 140, 140,
				&data.StringEntry{Key: "type", Value: "trade"},
				&data.StringEntry{Key: "side", Value: "buy"},
			)
			prior.At = now
			prior.From = now

			So(instrument.Step(prior.Write(
				data.NewMetric("price", 50001.0, data.UnitPrice, data.TimescaleInstantaneous),
			)), ShouldBeNil)
			So(instrument.Error(), ShouldNotBeNil)
		})

		Convey("Historical recurrence is undefined until every disposition z-score is", func() {
			touch(books, now, "bid-hist-1", 50000.0, 10.0, "ask-hist-1", 50002.0, 10.0)

			for step := range 3 {
				at := now.Add(time.Duration(step) * 100 * time.Millisecond)
				res := instrument.Step(trade(at, int64(201+step), "buy", 50002.0, float64(step+1)))
				So(res, ShouldNotBeNil)

				for _, label := range []string{"historical_path_distance", "historical_path_percentile"} {
					_, held := metric(res, label)
					So(held, ShouldBeFalse)
				}
			}
		})
	})

	Convey("A toxicity signal constructed without a book manager fails with an error", t, func() {
		instrument := toxicity.NewSignal(context.Background(), nil)
		So(instrument.Error(), ShouldNotBeNil)
		So(instrument.Status(), ShouldNotEqual, nmruntime.READY)
	})
}
