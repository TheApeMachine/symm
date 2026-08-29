package toxicity

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func trade(symbol string, side string, price float64, qty float64, at time.Time) kraken.TradeData {
	return kraken.TradeData{
		Symbol:    symbol,
		Side:      side,
		Price:     *decimal.NewFromFloat64(price),
		Qty:       qty,
		Timestamp: at,
	}
}

func TestTradeStep(t *testing.T) {
	Convey("Given a touch of 100/102", t, func() {
		entity := NewTrade()
		const bidPrice, askPrice, bidQty, askQty = 100.0, 102.0, 10.0, 20.0

		Convey("a sell at the bid touch attributes a fill", func() {
			measurement := entity.Step(trade("BTC/USD", "sell", 100, 3, time.Unix(1_700_000_001, 0)), bidPrice, askPrice, bidQty, askQty)

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["bracket_trade_quantity"].Raw, ShouldEqual, 3.0)
			So(measurement.Metrics["matched_touch_trade_quantity:bid"].Raw, ShouldEqual, 3.0)
			So(measurement.Metrics["matched_touch_trade_quantity:ask"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["touch_fill_quantity:bid"].Raw, ShouldEqual, 3.0)
			So(measurement.Metrics["touch_fill_fraction:bid"].Raw, ShouldAlmostEqual, 0.3, 1e-12)

			// The first trade has no spacing, so its fill rate is undefined.
			_, hasRate := measurement.Metrics["touch_fill_rate:bid"]
			So(hasRate, ShouldBeFalse)
		})

		Convey("a later matching trade accumulates the bracket and rate", func() {
			entity.Step(trade("BTC/USD", "sell", 100, 3, time.Unix(1_700_000_001, 0)), bidPrice, askPrice, bidQty, askQty)

			measurement := entity.Step(trade("BTC/USD", "sell", 100, 2, time.Unix(1_700_000_002, 0)), bidPrice, askPrice, bidQty, askQty)

			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["bracket_trade_quantity"].Raw, ShouldEqual, 5.0)
			So(measurement.Metrics["matched_touch_trade_quantity:bid"].Raw, ShouldEqual, 5.0)
			So(measurement.Metrics["touch_fill_quantity:bid"].Raw, ShouldEqual, 5.0)
			So(measurement.Metrics["touch_fill_fraction:bid"].Raw, ShouldAlmostEqual, 0.5, 1e-12)
			So(measurement.Metrics["touch_fill_rate:bid"].Raw, ShouldAlmostEqual, 5.0, 1e-12)
			So(measurement.Metrics["fill_fraction_baseline:bid"].Raw, ShouldNotEqual, 0.0)
			So(measurement.Metrics["fill_fraction_divergence:bid"].Raw, ShouldNotEqual, 0.0)
		})

		Convey("a buy away from the ask touch does not match", func() {
			measurement := entity.Step(trade("BTC/USD", "buy", 101, 4, time.Unix(1_700_000_001, 0)), bidPrice, askPrice, bidQty, askQty)

			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["matched_touch_trade_quantity:ask"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["touch_fill_quantity:ask"].Raw, ShouldEqual, 0.0)
		})
	})
}
