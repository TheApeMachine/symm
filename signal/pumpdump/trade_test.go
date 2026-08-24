package pumpdump

import (
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

		Convey("the opening trade seeds an open bar without fabricating rates", func() {
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
		})
	})
}
