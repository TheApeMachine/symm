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
	Convey("Given an executed-flow entity", t, func() {
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

			// Rates, velocities, baselines, and response-price metrics are
			// undefined until their prerequisites exist.
			_, hasTradeRate := measurement.Metrics["trade_rate"]
			_, hasBaseline := measurement.Metrics["signed_net_fraction_baseline"]
			_, hasMidpoint := measurement.Metrics["midpoint_log_return"]

			So(hasTradeRate, ShouldBeFalse)
			So(hasBaseline, ShouldBeFalse)
			So(hasMidpoint, ShouldBeFalse)
		})

		Convey("the first trade reports no SNR, its estimator having no baseline yet", func() {
			measurement := entity.Step(cvdTrade("BTC/USD", "buy", 100, 2, time.Unix(1000, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.SNRDefined, ShouldBeFalse)
		})

		Convey("a directional flow that keeps moving yields a defined SNR", func() {
			// Alternate the aggressor side so the signed net fraction actually
			// moves, which is what gives its estimator a noise model to report.
			for step := range 12 {
				side := "buy"

				if step%2 == 1 {
					side = "sell"
				}

				entity.Step(cvdTrade("BTC/USD", side, 100, 2, time.Unix(int64(1000+step), 0)))
			}

			measurement := entity.Step(cvdTrade("BTC/USD", "buy", 100, 5, time.Unix(1012, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.SNRDefined, ShouldBeTrue)
			So(measurement.SNR, ShouldBeGreaterThanOrEqualTo, 0)
		})

		Convey("a second sell trade advances accounting, rates, and baselines", func() {
			entity.Step(cvdTrade("BTC/USD", "buy", 100, 2, time.Unix(1000, 0)))
			measurement := entity.Step(cvdTrade("BTC/USD", "sell", 100, 1, time.Unix(1001, 0)))

			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["trade_count"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["trade_count:buy"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["trade_count:sell"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["signed_count_fraction"].Raw, ShouldEqual, 0.0)

			So(measurement.Metrics["gross_notional"].Raw, ShouldEqual, 300.0)
			So(measurement.Metrics["net_notional"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["signed_net_fraction"].Raw, ShouldAlmostEqual, 1.0/3.0, 1e-12)
			So(measurement.Metrics["mean_trade_notional"].Raw, ShouldEqual, 150.0)

			So(measurement.Metrics["trade_rate"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["gross_notional_rate"].Raw, ShouldEqual, 300.0)
			So(measurement.Metrics["net_notional_rate"].Raw, ShouldEqual, 100.0)

			// Causal directional baseline is the previous committed fraction;
			// divergence and z-score are judged against it.
			So(measurement.Metrics["signed_net_fraction_baseline"].Raw, ShouldEqual, 1.0)
			So(measurement.Metrics["signed_net_fraction_divergence"].Raw, ShouldAlmostEqual, -2.0/3.0, 1e-12)
			So(measurement.Metrics["signed_net_fraction_zscore"].Raw, ShouldEqual, -1.0)

			_, hasGrossBaseline := measurement.Metrics["gross_notional_rate_baseline"]

			So(hasGrossBaseline, ShouldBeFalse)
			So(measurement.Maturity, ShouldEqual, 0.5)
		})

		Convey("a third buy trade advances baselines, velocities, and response", func() {
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

			// This entity has no access to book state, so the response-price
			// family (midpoint_*, flow_aligned_*) never populates.
			_, hasMidpoint := measurement.Metrics["midpoint_log_return"]
			So(hasMidpoint, ShouldBeFalse)
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

/*
TestResponsePriceWithQuote proves the restored quote path: with a shared quote
provider, the response-price metrics (midpoint and midpoint_log_return) become
computable from real bid/ask, so the causal outcome coordinate the decision
loop depends on is no longer permanently undefined. A second causally ordered
quote yields a defined midpoint_log_return.
*/
func TestResponsePriceWithQuote(t *testing.T) {
	Convey("Given a Trade entity with a moving quote provider", t, func() {
		entity := NewTrade()
		quote := 100.0

		entity.SetQuote(func(symbol string) (bid, ask *decimal.Decimal) {
			bidValue := decimal.NewFromFloat64(quote)
			askValue := decimal.NewFromFloat64(quote + 1.0)

			return bidValue, askValue
		})

		Convey("the first trade has no prior quote, so the midpoint is undefined", func() {
			measurement := entity.Step(cvdTrade("BTC/USD", "buy", 100, 1, time.Unix(1000, 0)))

			So(measurement, ShouldNotBeNil)
			_, hasMidpoint := measurement.Metrics["midpoint_log_return"]
			So(hasMidpoint, ShouldBeFalse)
		})

		Convey("a second trade with a moved quote yields a defined midpoint log return", func() {
			entity.Step(cvdTrade("BTC/USD", "buy", 100, 1, time.Unix(1000, 0)))

			// Move the quote up: the midpoint at the second observation is
			// higher than the prior retained midpoint, so midpoint_log_return
			// is a defined non-zero value.
			quote = 110.0

			measurement := entity.Step(cvdTrade("BTC/USD", "buy", 101, 1, time.Unix(1001, 0)))

			So(measurement, ShouldNotBeNil)
			midpoint, hasMidpoint := measurement.Metrics["midpoint_log_return"]
			So(hasMidpoint, ShouldBeTrue)
			So(midpoint.Raw, ShouldNotEqual, 0.0)
		})
	})
}
