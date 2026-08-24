package derivatives

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func futuresTrade(
	symbol string,
	price float64,
	qty float64,
	side string,
	tradeType string,
	at time.Time,
) kraken.FuturesTradeData {
	return kraken.FuturesTradeData{
		Symbol:    symbol,
		Price:     *decimal.NewFromFloat64(price),
		Qty:       qty,
		Side:      side,
		Type:      tradeType,
		Timestamp: at,
	}
}

func TestTradeStep(t *testing.T) {
	Convey("Given a multi-leg liquidation sequence", t, func() {
		entity := NewTrade()
		at := time.Unix(1_700_000_000, 0)

		Convey("a single buy liquidation accounts its interval", func() {
			measurement := entity.Step(futuresTrade("PF_XBTUSD", 100, 2, "buy", "liquidation", at))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["liquidation_notional:buy"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["liquidation_notional:sell"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["gross_liquidation_notional"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["net_liquidation_notional"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["liquidation_signed_fraction"].Raw, ShouldAlmostEqual, 1.0, 1e-12)
			So(measurement.Metrics["gross_derivative_trade_notional"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["liquidation_share"].Raw, ShouldAlmostEqual, 1.0, 1e-12)

			// The first trade opens the interval: no positive duration yet.
			_, hasRate := measurement.Metrics["liquidation_notional_rate"]
			So(hasRate, ShouldBeFalse)

			// One retained trade is still immature support.
			So(measurement.Maturity, ShouldEqual, 0.0)
		})

		Convey("a follow-up sell liquidation extends the interval", func() {
			entity.Step(futuresTrade("PF_XBTUSD", 100, 2, "buy", "liquidation", at))
			measurement := entity.Step(futuresTrade("PF_XBTUSD", 110, 1, "sell", "liquidation", at.Add(5*time.Second)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["liquidation_notional:buy"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["liquidation_notional:sell"].Raw, ShouldEqual, 110.0)
			So(measurement.Metrics["gross_liquidation_notional"].Raw, ShouldEqual, 310.0)
			So(measurement.Metrics["net_liquidation_notional"].Raw, ShouldEqual, 90.0)
			So(measurement.Metrics["liquidation_signed_fraction"].Raw, ShouldAlmostEqual, 90.0/310.0, 1e-12)
			So(measurement.Metrics["liquidation_notional_rate"].Raw, ShouldAlmostEqual, 310.0/5.0, 1e-12)
			So(measurement.Metrics["gross_derivative_trade_notional"].Raw, ShouldEqual, 310.0)
			So(measurement.Metrics["liquidation_share"].Raw, ShouldAlmostEqual, 1.0, 1e-12)
		})
	})

	Convey("Given a non-liquidation trade", t, func() {
		entity := NewTrade()

		Convey("gross liquidation is a valid zero and the signed fraction is omitted", func() {
			measurement := entity.Step(futuresTrade("PF_XBTUSD", 100, 2, "buy", "trade", time.Unix(1_700_000_000, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["gross_liquidation_notional"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["net_liquidation_notional"].Raw, ShouldEqual, 0.0)
			_, hasFraction := measurement.Metrics["liquidation_signed_fraction"]
			So(hasFraction, ShouldBeFalse)
			So(measurement.Metrics["liquidation_share"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["gross_derivative_trade_notional"].Raw, ShouldEqual, 200.0)
		})
	})
}
