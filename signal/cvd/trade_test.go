package cvd

import (
	"math"
	"maps"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
schema is the register's declared metric set the workload's data management
hands the signal: every producible metric, none valued.
*/
var schema = new(Trade).Register().Metrics

/*
row builds the measurement a trade row lifts into: the feed fills the price
and quantity metrics, carries the categorical aggressor side in provenance,
and names the symbol and venue timestamp. Zero or negative price/quantity is
an invalid execution.
*/
func row(symbol, side string, price, qty float64, at time.Time) *data.Measurement[float64] {
	m := data.NewMeasurement[float64]("websocket", maps.Clone(schema))
	m.Label, m.At, m.From = symbol, at, at
	m.Metrics["price"] = m.Metrics["price"].Write(price)
	m.Metrics["qty"] = m.Metrics["qty"].Write(qty)
	m.Provenance = map[string]string{"side": side}

	return m
}

func timestamp(second int64) time.Time {
	return time.Unix(1_000+second, 0)
}

func TestTradeStep(t *testing.T) {
	Convey("Given an executed-flow entity", t, func() {
		entity := NewTrade(t.Context())

		Convey("the first buy trade yields a measurement with no warmup gating", func() {
			measurement := entity.Step(row("BTC/USD", "buy", 100, 2, timestamp(0)))

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
			measurement := entity.Step(row("BTC/USD", "buy", 100, 2, timestamp(0)))

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

				entity.Step(row("BTC/USD", side, 100, 2, timestamp(int64(step))))
			}

			measurement := entity.Step(row("BTC/USD", "buy", 100, 5, timestamp(12)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.SNRDefined, ShouldBeTrue)
			So(measurement.SNR, ShouldBeGreaterThanOrEqualTo, 0)
		})

		Convey("a second sell trade advances accounting, rates, and baselines", func() {
			entity.Step(row("BTC/USD", "buy", 100, 2, timestamp(0)))
			measurement := entity.Step(row("BTC/USD", "sell", 100, 1, timestamp(1)))

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
			// The z-score is judged against the prior dispersion of the two
			// committed fractions (1 and 1/3), not a single-sample fallback.
			So(measurement.Metrics["signed_net_fraction_zscore"].Raw, ShouldAlmostEqual, -math.Sqrt(2.0), 1e-12)

			So(measurement.Maturity, ShouldEqual, 0.5)
		})

		Convey("a third buy trade advances baselines, velocities, and response", func() {
			entity.Step(row("BTC/USD", "buy", 100, 2, timestamp(0)))
			entity.Step(row("BTC/USD", "sell", 100, 1, timestamp(1)))

			measurement := entity.Step(row("BTC/USD", "buy", 100, 1, timestamp(3)))

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
		entity := NewTrade(t.Context())

		Convey("the measurement carries the pipeline rejection in its Err field", func() {
			measurement := entity.Step(row("BTC/USD", "buy", 0, 1, timestamp(0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})

	Convey("Given a trade whose side is not a known aggressor", t, func() {
		entity := NewTrade(t.Context())

		Convey("the measurement carries the pipeline rejection in its Err field", func() {
			measurement := entity.Step(row("BTC/USD", "both", 100, 1, timestamp(0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})
}

/*
TestTradeRegister proves the declared schema: every producible metric is
declared, none valued, and every label names itself.
*/
func TestTradeRegister(t *testing.T) {
	Convey("Given a Trade entity", t, func() {
		entity := new(Trade)

		Convey("Register declares the full metric schema without values", func() {
			measurement := entity.Register()

			So(measurement.ID, ShouldEqual, -1)
			So(measurement.Metrics, ShouldContainKey, "trade_count")
			So(measurement.Metrics, ShouldContainKey, "cumulative_volume_delta")
			So(measurement.Metrics, ShouldContainKey, "signed_net_fraction_zscore")

			for label, metric := range measurement.Metrics {
				So(label, ShouldEqual, metric.Label)
				So(metric.Raw, ShouldEqual, 0.0)
			}
		})
	})
}
