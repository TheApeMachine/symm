package pumpdump

import (
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
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
	Convey("Given a multi-leg volume-clock sequence", t, func() {
		entity := NewTrade()
		at := time.Unix(1_700_000_000, 0)

		Convey("the opening trade seeds an open bar", func() {
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

			// This entity has no access to book state, so the midpoint
			// response family never populates.
			_, hasMidpoint := measurement.Metrics["midpoint"]
			So(hasMidpoint, ShouldBeFalse)

			// No previous trade exists yet, so the trade interval is absent.
			_, hasInterval := measurement.Metrics["trade_interval_seconds"]
			So(hasInterval, ShouldBeFalse)
		})

		Convey("the closing trade reports the completed bar and its throughput", func() {
			entity.Step(spotTrade("BTC/USD", 100, 2, at))
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

			// This entity has no access to book state, so the midpoint
			// response family never populates.
			_, hasMidpoint := measurement.Metrics["midpoint"]
			So(hasMidpoint, ShouldBeFalse)
		})
	})

	Convey("Given completed volume bars backed by executable quotes", t, func() {
		entity := NewTrade()
		midpoint := 100.0
		at := time.Unix(1_700_000_000, 0)

		entity.SetQuote(func(symbol string) (bid, ask *decimal.Decimal) {
			bidValue := decimal.NewFromFloat64(midpoint - 1)
			askValue := decimal.NewFromFloat64(midpoint + 1)

			return bidValue, askValue
		})

		Convey("the ordinal advances only when a bar closes and the midpoint records downside then recovery", func() {
			opening := entity.Step(spotTrade("BTC/USD", 100, 2, at))
			So(opening.Err, ShouldBeNil)
			So(opening.Metrics["completed_volume_bar_ordinal"].Raw, ShouldEqual, 0.0)

			midpoint = 90
			downside := entity.Step(spotTrade(
				"BTC/USD", 90, 1, at.Add(5*time.Second),
			))
			downsideReturn := math.Log(90.0 / 100.0)

			So(downside.Err, ShouldBeNil)
			So(downside.Metrics["completed_volume_bar_ordinal"].Raw, ShouldEqual, 1.0)
			So(downside.Metrics["midpoint:from"].Raw, ShouldEqual, 100.0)
			So(downside.Metrics["midpoint:at"].Raw, ShouldEqual, 90.0)
			So(downside.Metrics["midpoint_log_return"].Raw, ShouldAlmostEqual, downsideReturn, 1e-12)
			So(downside.Metrics["negative_midpoint_return"].Raw, ShouldAlmostEqual, -downsideReturn, 1e-12)
			So(downside.Metrics["positive_midpoint_return"].Raw, ShouldEqual, 0.0)

			barOpening := entity.Step(spotTrade(
				"BTC/USD", 90, 0.25, at.Add(6*time.Second),
			))
			So(barOpening.Err, ShouldBeNil)
			So(barOpening.Metrics["completed_volume_bar_ordinal"].Raw, ShouldEqual, 1.0)

			midpoint = 95
			insideBar := entity.Step(spotTrade(
				"BTC/USD", 95, 0.25, at.Add(7*time.Second),
			))
			So(insideBar.Err, ShouldBeNil)
			So(insideBar.Metrics["completed_volume_bar_ordinal"].Raw, ShouldEqual, 1.0)
			_, hasIntraBarReturn := insideBar.Metrics["midpoint_log_return"]
			So(hasIntraBarReturn, ShouldBeFalse)

			midpoint = 105
			recovery := entity.Step(spotTrade(
				"BTC/USD", 105, 1, at.Add(10*time.Second),
			))
			recoveryReturn := math.Log(105.0 / 90.0)

			So(recovery.Err, ShouldBeNil)
			So(recovery.Metrics["completed_volume_bar_ordinal"].Raw, ShouldEqual, 2.0)
			So(recovery.Metrics["midpoint:from"].Raw, ShouldEqual, 90.0)
			So(recovery.Metrics["midpoint:at"].Raw, ShouldEqual, 105.0)
			So(recovery.Metrics["midpoint_log_return"].Raw, ShouldAlmostEqual, recoveryReturn, 1e-12)
			So(recovery.Metrics["positive_midpoint_return"].Raw, ShouldAlmostEqual, recoveryReturn, 1e-12)
			So(recovery.Metrics["negative_midpoint_return"].Raw, ShouldEqual, 0.0)
			So(recovery.Metrics["midpoint_return_velocity"].Raw, ShouldAlmostEqual, recoveryReturn-downsideReturn, 1e-12)
		})
	})
}

func BenchmarkTradeStep(b *testing.B) {
	entity := NewTrade()
	quotes := [2]struct {
		bid *decimal.Decimal
		ask *decimal.Decimal
	}{
		{bid: decimal.NewFromFloat64(99), ask: decimal.NewFromFloat64(101)},
		{bid: decimal.NewFromFloat64(100), ask: decimal.NewFromFloat64(102)},
	}
	quoteIndex := 0

	entity.SetQuote(func(symbol string) (bid, ask *decimal.Decimal) {
		return quotes[quoteIndex].bid, quotes[quoteIndex].ask
	})

	b.ReportAllocs()
	

	for iteration := 0; b.Loop(); iteration++ {
		quoteIndex = iteration % len(quotes)
		measurement := entity.Step(spotTrade(
			"BTC/USD",
			100+float64(quoteIndex),
			1,
			time.Unix(1_700_000_000+int64(iteration), 0),
		))

		if measurement.Err != nil {
			b.Fatal(measurement.Err)
		}
	}
}
