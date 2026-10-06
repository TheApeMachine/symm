package pumpdump_test

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
	"github.com/theapemachine/symm/signal/pumpdump"
)

func trade(at time.Time, seq int64, side string, price, qty float64) *data.Measurement {
	prior := data.NewMeasurement(
		1, "BTC/USD", "ingress", seq, seq,
		data.StringEntry{Key: "side", Value: side},
		data.StringEntry{Key: "channel", Value: "trade"},
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

func TestPumpDumpSignal(t *testing.T) {
	Convey("Volume-clocked activity instrument measures tape, touch, and midpoint response", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner("pumpdump", 4096)
		now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		books := broker.NewBook(ctx, spot.NewNormalizer())

		instrument := pumpdump.NewSignal(ctx, arena, books)
		instrument.Transition(nmruntime.READY)

		Convey("Volume clock aggregates exact trade bars, durations, and volume/notional rates", func() {
			touch(books, now, "bid-1", 50000.0, 5.0, "ask-1", 50002.0, 5.0)

			res1 := instrument.Step(trade(now, 1, "buy", 50001.0, 1.0))
			So(res1, ShouldNotBeNil)
			So(metricValue(res1, "trade_price"), ShouldEqual, 50001.0)
			So(metricValue(res1, "trade_quantity"), ShouldEqual, 1.0)
			So(metricValue(res1, "trade_notional"), ShouldEqual, 50001.0)
			So(metricValue(res1, "midpoint"), ShouldEqual, 50001.0)
			So(metricValue(res1, "spread"), ShouldEqual, 2.0)
			So(metricValue(res1, "relative_spread"), ShouldAlmostEqual, 2.0/50001.0, 1e-12)

			// An open bar is not a zero-rate bar.
			_, hasRate := metric(res1, "volume_rate")
			So(hasRate, ShouldBeFalse)
			_, hasInterval := metric(res1, "trade_interval_seconds")
			So(hasInterval, ShouldBeFalse)

			// 200ms later: bar quantity 2.0 reaches the bootstrap target 1.0 and closes.
			res2 := instrument.Step(trade(now.Add(200*time.Millisecond), 2, "buy", 50002.0, 1.0))
			So(res2, ShouldNotBeNil)
			So(res2.From, ShouldEqual, now)
			So(instrument.Error(), ShouldBeNil)

			So(metricValue(res2, "trade_interval_seconds"), ShouldAlmostEqual, 0.2, 1e-9)
			So(metricValue(res2, "volume_bar_target_quantity"), ShouldAlmostEqual, 1.0, 1e-9)
			So(metricValue(res2, "volume_bar_quantity"), ShouldAlmostEqual, 2.0, 1e-9)
			So(metricValue(res2, "volume_bar_notional"), ShouldAlmostEqual, 50001.0+50002.0, 1e-6)
			So(metricValue(res2, "volume_bar_trade_count"), ShouldEqual, 2)
			So(metricValue(res2, "volume_bar_duration"), ShouldAlmostEqual, 0.2, 1e-9)
			So(metricValue(res2, "volume_rate"), ShouldAlmostEqual, 2.0/0.2, 1e-6)
			So(metricValue(res2, "notional_rate"), ShouldAlmostEqual, 100003.0/0.2, 1e-6)
			So(metricValue(res2, "trade_rate"), ShouldAlmostEqual, 2.0/0.2, 1e-6)
			So(metricValue(res2, "completed_bars"), ShouldEqual, 1)

			// Unchanged touch: a valid zero midpoint return.
			So(metricValue(res2, "midpoint:from"), ShouldEqual, 50001.0)
			So(metricValue(res2, "midpoint:at"), ShouldEqual, 50001.0)
			So(metricValue(res2, "midpoint_log_return"), ShouldEqual, 0.0)
		})

		Convey("Pump: activity surge and spread blowout yield positive divergences and an outlier z-score", func() {
			basePrice := 50000.0

			for step := 0; step < 12; step++ {
				at := now.Add(time.Duration(step*100) * time.Millisecond)
				touch(books, at, "bid-calm", basePrice-1.0, 5.0, "ask-calm", basePrice+1.0, 5.0)
				So(instrument.Step(trade(at, int64(step+10), "buy", basePrice, 0.5)), ShouldNotBeNil)
			}

			pumpAt := now.Add(1300 * time.Millisecond)
			touch(books, pumpAt, "bid-pump", basePrice, 1.0, "ask-pump", basePrice+40.0, 1.0)

			pumpRes := instrument.Step(trade(pumpAt, 25, "buy", basePrice+35.0, 10.0))
			So(pumpRes, ShouldNotBeNil)
			So(instrument.Error(), ShouldBeNil)

			So(metricValue(pumpRes, "spread_divergence"), ShouldBeGreaterThan, 0.0)
			So(metricValue(pumpRes, "spread_ratio"), ShouldBeGreaterThan, 1.0)
			So(metricValue(pumpRes, "spread_zscore"), ShouldBeGreaterThan, 2.0)
			So(metricValue(pumpRes, "notional_rate_divergence"), ShouldBeGreaterThan, 0.0)
			So(metricValue(pumpRes, "notional_rate_ratio"), ShouldBeGreaterThan, 1.0)
			So(metricValue(pumpRes, "positive_midpoint_return"), ShouldBeGreaterThan, 0.0)
			So(metricValue(pumpRes, "negative_midpoint_return"), ShouldEqual, 0.0)
		})

		Convey("Dump: a midpoint crash yields a negative return decomposed into r⁺ - r⁻", func() {
			basePrice := 50000.0

			for step := 0; step < 5; step++ {
				at := now.Add(time.Duration(step*100) * time.Millisecond)
				touch(books, at, "bid-calm", basePrice-2.0, 5.0, "ask-calm", basePrice+2.0, 5.0)
				So(instrument.Step(trade(at, int64(step+30), "sell", basePrice, 1.0)), ShouldNotBeNil)
			}

			dumpAt := now.Add(600 * time.Millisecond)
			touch(books, dumpAt, "bid-dump", basePrice-52.0, 5.0, "ask-dump", basePrice-48.0, 5.0)

			dumpRes := instrument.Step(trade(dumpAt, 36, "sell", basePrice-50.0, 3.0))
			So(dumpRes, ShouldNotBeNil)
			So(instrument.Error(), ShouldBeNil)

			logReturn := math.Log((basePrice - 50.0) / basePrice)
			So(metricValue(dumpRes, "midpoint_log_return"), ShouldAlmostEqual, logReturn, 1e-12)
			So(metricValue(dumpRes, "midpoint_return_rate"), ShouldAlmostEqual, logReturn/0.2, 1e-9)
			So(metricValue(dumpRes, "positive_midpoint_return"), ShouldEqual, 0.0)
			So(metricValue(dumpRes, "negative_midpoint_return"), ShouldAlmostEqual, -logReturn, 1e-12)
		})

		Convey("A crossed touch advances tape accounting but omits every touch-dependent fact", func() {
			touch(books, now, "bid-crossed", 50010.0, 1.0, "ask-crossed", 50000.0, 1.0)

			res := instrument.Step(trade(now, 50, "buy", 50005.0, 1.0))
			So(res, ShouldNotBeNil)
			So(metricValue(res, "trade_notional"), ShouldEqual, 50005.0)

			for _, label := range []string{"best_bid", "best_ask", "midpoint", "spread", "relative_spread"} {
				_, held := metric(res, label)
				So(held, ShouldBeFalse)
			}
		})

		Convey("A trade without a positive quantity yields no measurement", func() {
			touch(books, now, "bid-1", 50000.0, 5.0, "ask-1", 50002.0, 5.0)
			So(instrument.Step(trade(now, 60, "buy", 50001.0, 0.0)), ShouldBeNil)
		})
	})
}
