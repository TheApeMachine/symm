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

			So(metricValue(res1, "bracket_trade_quantity"), ShouldAlmostEqual, trade1Qty, 1e-9)
			So(metricValue(res1, "matched_touch_trade_quantity:ask"), ShouldAlmostEqual, trade1Qty, 1e-9)
			So(metricValue(res1, "touch_fill_quantity:ask"), ShouldAlmostEqual, trade1Qty, 1e-9)
			So(metricValue(res1, "touch_fill_fraction:ask"), ShouldAlmostEqual, trade1Qty/touchQty, 1e-9)

			So(metricValue(res1, "matched_touch_trade_quantity:bid"), ShouldAlmostEqual, 0.0, 1e-9)
			So(metricValue(res1, "touch_fill_quantity:bid"), ShouldAlmostEqual, 0.0, 1e-9)
			So(metricValue(res1, "touch_fill_fraction:bid"), ShouldAlmostEqual, 0.0, 1e-9)

			So(metricValue(res1, "previous_best_price:bid"), ShouldEqual, 0.0)
			So(metricValue(res1, "touch_fill_rate:ask"), ShouldEqual, 0.0)

			trade2Qty := 2.5
			trade2At := now.Add(100 * time.Millisecond)
			res2 := instrument.Step(trade(trade2At, 2, "buy", askPrice, trade2Qty))
			So(res2, ShouldNotBeNil)
			So(instrument.Error(), ShouldBeNil)

			expectedCumQty := trade1Qty + trade2Qty
			So(metricValue(res2, "touch_fill_quantity:ask"), ShouldAlmostEqual, expectedCumQty, 1e-9)
			So(metricValue(res2, "touch_fill_fraction:ask"), ShouldAlmostEqual, expectedCumQty/touchQty, 1e-9)
			So(metricValue(res2, "touch_fill_rate:ask"), ShouldAlmostEqual, expectedCumQty/0.1, 1e-6)
			So(res2.From, ShouldEqual, now)

			// A trade outside the bracket neither brackets nor matches.
			res3 := instrument.Step(trade(trade2At.Add(100*time.Millisecond), 3, "buy", askPrice+5, 1.0))
			So(res3, ShouldNotBeNil)
			So(metricValue(res3, "bracket_trade_quantity"), ShouldAlmostEqual, expectedCumQty, 1e-9)
			So(metricValue(res3, "touch_fill_quantity:ask"), ShouldAlmostEqual, expectedCumQty, 1e-9)
			So(metricValue(res3, "touch_fill_fraction:ask"), ShouldAlmostEqual, 0.0, 1e-9)
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

			// A retreating bid is not a withdrawal; the unchanged ask neither retreats nor withdraws.
			for _, label := range []string{
				"net_withdrawn_quantity:bid", "retreat_fraction:ask", "retreated_quantity:ask",
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

		Convey("Historical recurrence calculates trajectory distance and empirical percentile", func() {
			bidPrice, askPrice, touchQty := 50000.0, 50002.0, 10.0
			touch(books, now, "bid-hist-1", bidPrice, touchQty, "ask-hist-1", askPrice, touchQty)

			res1 := instrument.Step(trade(now, 201, "buy", askPrice, 1.0))
			So(res1, ShouldNotBeNil)
			So(metricValue(res1, "historical_path_distance"), ShouldEqual, 0.0)
			So(metricValue(res1, "historical_path_percentile"), ShouldEqual, 0.0)

			step2At := now.Add(100 * time.Millisecond)
			res2 := instrument.Step(trade(step2At, 202, "buy", askPrice, 3.0))
			So(res2, ShouldNotBeNil)
			So(metricValue(res2, "historical_path_distance"), ShouldBeGreaterThan, 0.0)
			So(metricValue(res2, "historical_path_percentile"), ShouldEqual, 0.0)

			step3At := step2At.Add(100 * time.Millisecond)
			res3 := instrument.Step(trade(step3At, 203, "buy", askPrice, 3.0))
			So(res3, ShouldNotBeNil)
			dist3 := metricValue(res3, "historical_path_distance")
			perc3 := metricValue(res3, "historical_path_percentile")
			So(dist3, ShouldBeGreaterThanOrEqualTo, 0.0)
			So(perc3, ShouldBeGreaterThanOrEqualTo, 0.0)
			So(perc3, ShouldBeLessThanOrEqualTo, 1.0)
		})
	})

	Convey("A toxicity signal constructed without a book manager fails with an error", t, func() {
		instrument := toxicity.NewSignal(context.Background(), nil)
		So(instrument.Error(), ShouldNotBeNil)
		So(instrument.Status(), ShouldNotEqual, nmruntime.READY)
	})
}
