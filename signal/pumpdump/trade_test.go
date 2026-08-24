package pumpdump

import (
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func spotTrade(symbol string, price float64, qty float64, at time.Time) kraken.TradeData {
	return kraken.TradeData{
		Symbol:    symbol,
		Price:     *decimal.NewFromFloat64(price),
		Qty:       qty,
		Timestamp: at,
	}
}

func TestTradeStep(t *testing.T) {
	Convey("Given a multi-leg volume-clock sequence with a shared book", t, func() {
		workspace := runtime.NewWorkspace(nil)
		workspace.Share("book", populatedBook(99, 1, 101, 1), "BTC/USD")
		entity := NewTrade(workspace)
		at := time.Unix(1_700_000_000, 0)

		Convey("the opening trade seeds an open bar and latches the opening midpoint", func() {
			measurement := entity.Step(spotTrade("BTC/USD", 100, 2, at))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["trade_price"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["trade_quantity"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["trade_notional"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["volume_bar_target_quantity"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["volume_bar_quantity"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["volume_bar_notional"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["volume_bar_trade_count"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["volume_bar_duration"].Raw, ShouldEqual, 0.0)

			// An incomplete bar is not a zero-rate bar: rates are absent.
			_, hasVolumeRate := measurement.Metrics["volume_rate"]
			So(hasVolumeRate, ShouldBeFalse)
			_, hasNotionalRate := measurement.Metrics["notional_rate"]
			So(hasNotionalRate, ShouldBeFalse)

			// The bar-opening midpoint is latched from the book touch.
			So(measurement.Metrics["midpoint:from"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["midpoint:at"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["midpoint"].Raw, ShouldEqual, 100.0)

			// No previous trade exists yet, so the trade interval is absent.
			_, hasInterval := measurement.Metrics["trade_interval_seconds"]
			So(hasInterval, ShouldBeFalse)
		})

		Convey("the closing trade reports the completed bar, its throughput, and the midpoint response", func() {
			entity.Step(spotTrade("BTC/USD", 100, 2, at))
			workspace.Share("book", populatedBook(104, 1, 106, 1), "BTC/USD")
			measurement := entity.Step(spotTrade("BTC/USD", 110, 1, at.Add(5*time.Second)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["volume_bar_target_quantity"].Raw, ShouldAlmostEqual, 1.5, 1e-12)
			So(measurement.Metrics["volume_bar_quantity"].Raw, ShouldEqual, 3.0)
			So(measurement.Metrics["volume_bar_notional"].Raw, ShouldEqual, 310.0)
			So(measurement.Metrics["volume_bar_trade_count"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["volume_bar_duration"].Raw, ShouldEqual, 5.0)

			So(measurement.Metrics["volume_rate"].Raw, ShouldAlmostEqual, 3.0/5.0, 1e-12)
			So(measurement.Metrics["notional_rate"].Raw, ShouldAlmostEqual, 310.0/5.0, 1e-12)
			So(measurement.Metrics["trade_rate"].Raw, ShouldAlmostEqual, 2.0/5.0, 1e-12)
			So(measurement.Metrics["trade_interval_seconds"].Raw, ShouldAlmostEqual, 5.0, 1e-12)

			// The notional-rate baseline of one value is the value itself.
			So(measurement.Metrics["notional_rate_baseline"].Raw, ShouldAlmostEqual, 310.0/5.0, 1e-9)
			So(measurement.Metrics["notional_rate_ratio"].Raw, ShouldAlmostEqual, 1.0, 1e-9)

			// The midpoint response is log(close midpoint / open midpoint).
			So(measurement.Metrics["midpoint:from"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["midpoint:at"].Raw, ShouldEqual, 105.0)
			So(measurement.Metrics["midpoint_log_return"].Raw, ShouldAlmostEqual, math.Log(105.0/100.0), 1e-12)
			So(measurement.Metrics["midpoint_return_rate"].Raw, ShouldAlmostEqual, math.Log(105.0/100.0)/5.0, 1e-12)
			So(measurement.Metrics["positive_midpoint_return"].Raw, ShouldAlmostEqual, math.Log(105.0/100.0), 1e-12)
			So(measurement.Metrics["negative_midpoint_return"].Raw, ShouldAlmostEqual, 0.0, 1e-12)

			// The return baseline of one value is the value itself.
			So(measurement.Metrics["midpoint_return_baseline"].Raw, ShouldAlmostEqual, math.Log(105.0/100.0), 1e-9)
		})
	})

	Convey("Given a trade without a shared book", t, func() {
		entity := NewTrade(runtime.NewWorkspace(nil))
		at := time.Unix(1_700_000_000, 0)

		Convey("touch-dependent metrics are omitted without fabricating a midpoint", func() {
			measurement := entity.Step(spotTrade("BTC/USD", 100, 2, at))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["trade_notional"].Raw, ShouldEqual, 200.0)
			_, hasMidpoint := measurement.Metrics["midpoint"]
			So(hasMidpoint, ShouldBeFalse)
			_, hasFrom := measurement.Metrics["midpoint:from"]
			So(hasFrom, ShouldBeFalse)
		})
	})
}
