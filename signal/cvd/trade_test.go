package cvd

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func cvdTrade(symbol string, side string, price float64, qty float64, at time.Time) kraken.TradeData {
	return kraken.TradeData{
		Symbol:    symbol,
		Side:      side,
		Price:     *decimal.NewFromFloat64(price),
		Qty:       qty,
		Timestamp: at,
	}
}

func TestTradeStep(t *testing.T) {
	Convey("Given a fresh executed-flow entity", t, func() {
		entity := NewTrade()

		Convey("the first buy trade yields a measurement with no warmup gating", func() {
			measurement := entity.Step(cvdTrade("BTC/USD", "buy", 100, 2, time.Unix(1000, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["trade_count"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["trade_count:buy"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["trade_count:sell"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["signed_count_fraction"].Raw, ShouldEqual, 1.0)

			So(measurement.Metrics["executed_quantity:buy"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["executed_quantity:sell"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["gross_executed_quantity"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["net_executed_quantity"].Raw, ShouldEqual, 2.0)

			So(measurement.Metrics["aggressive_notional:buy"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["aggressive_notional:sell"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["gross_notional"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["net_notional"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["signed_net_fraction"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["mean_trade_notional"].Raw, ShouldEqual, 200.0)

			So(measurement.Metrics["cumulative_volume_delta"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["cumulative_notional_delta"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["cvd_epoch_from"].Raw, ShouldEqual, 1000.0)

			// One trade carries a single effective observation: Maturity 0.
			So(measurement.Maturity, ShouldEqual, 0.0)

			// Rates are undefined until a positive interval elapses; baselines
			// are undefined until their own causal history exists.
			_, hasTradeRate := measurement.Metrics["trade_rate"]
			_, hasBaseline := measurement.Metrics["signed_net_fraction_baseline"]

			So(hasTradeRate, ShouldBeFalse)
			So(hasBaseline, ShouldBeFalse)
		})

		Convey("a second sell trade advances the accounting and emits rates", func() {
			entity.Step(cvdTrade("BTC/USD", "buy", 100, 2, time.Unix(1000, 0)))
			measurement := entity.Step(cvdTrade("BTC/USD", "sell", 100, 1, time.Unix(1001, 0)))

			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["trade_count"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["trade_count:buy"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["trade_count:sell"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["signed_count_fraction"].Raw, ShouldEqual, 0.0)

			So(measurement.Metrics["gross_executed_quantity"].Raw, ShouldEqual, 3.0)
			So(measurement.Metrics["net_executed_quantity"].Raw, ShouldEqual, 1.0)

			So(measurement.Metrics["aggressive_notional:buy"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["aggressive_notional:sell"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["gross_notional"].Raw, ShouldEqual, 300.0)
			So(measurement.Metrics["net_notional"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["signed_net_fraction"].Raw, ShouldAlmostEqual, 1.0/3.0, 1e-12)
			So(measurement.Metrics["mean_trade_notional"].Raw, ShouldEqual, 150.0)

			So(measurement.Metrics["trade_rate"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["gross_notional_rate"].Raw, ShouldEqual, 300.0)
			So(measurement.Metrics["net_notional_rate"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["buy_notional_rate"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["sell_notional_rate"].Raw, ShouldEqual, 100.0)

			// Causal directional baseline is the mean of the one prior fraction.
			So(measurement.Metrics["signed_net_fraction_baseline"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["signed_net_fraction_divergence"].Raw, ShouldAlmostEqual, -2.0/3.0, 1e-12)

			_, hasGrossBaseline := measurement.Metrics["gross_notional_rate_baseline"]

			So(hasGrossBaseline, ShouldBeFalse)
			So(measurement.Maturity, ShouldEqual, 0.5)
		})

		Convey("a third buy trade advances both causal baselines", func() {
			entity.Step(cvdTrade("BTC/USD", "buy", 100, 2, time.Unix(1000, 0)))
			entity.Step(cvdTrade("BTC/USD", "sell", 100, 1, time.Unix(1001, 0)))
			measurement := entity.Step(cvdTrade("BTC/USD", "buy", 100, 1, time.Unix(1003, 0)))

			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["trade_count"].Raw, ShouldEqual, 3.0)
			So(measurement.Metrics["trade_count:buy"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["trade_count:sell"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["signed_count_fraction"].Raw, ShouldAlmostEqual, 1.0/3.0, 1e-12)

			So(measurement.Metrics["gross_notional"].Raw, ShouldEqual, 400.0)
			So(measurement.Metrics["net_notional"].Raw, ShouldEqual, 200.0)
			So(measurement.Metrics["signed_net_fraction"].Raw, ShouldEqual, 0.5)

			So(measurement.Metrics["gross_notional_rate"].Raw, ShouldAlmostEqual, 400.0/3.0, 1e-9)
			So(measurement.Metrics["signed_net_fraction_baseline"].Raw, ShouldAlmostEqual, 2.0/3.0, 1e-12)
			So(measurement.Metrics["signed_net_fraction_divergence"].Raw, ShouldAlmostEqual, -1.0/6.0, 1e-12)

			So(measurement.Metrics["gross_notional_rate_baseline"].Raw, ShouldEqual, 300.0)
			So(measurement.Metrics["gross_notional_rate_ratio"].Raw, ShouldAlmostEqual, 4.0/9.0, 1e-12)
			So(measurement.Metrics["gross_notional_rate_divergence"].Raw, ShouldAlmostEqual, -500.0/3.0, 1e-9)

			So(measurement.Maturity, ShouldAlmostEqual, 2.0/3.0, 1e-12)
		})
	})

	Convey("Given a non-positive execution price", t, func() {
		entity := NewTrade()

		Convey("the measurement carries the pipeline rejection in its Err field", func() {
			measurement := entity.Step(cvdTrade("BTC/USD", "buy", 0, 1, time.Unix(1000, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})
}
