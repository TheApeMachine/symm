package audit

import (
	"context"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestTruthfulnessStage(t *testing.T) {
	Convey("Given a TruthfulnessStage instance", t, func() {
		ctx := context.Background()
		stage := NewTruthfulnessStage(ctx)

		feed := func(s *TruthfulnessStage, ms ...*data.Measurement) []*Violation {
			for _, m := range ms {
				s.Channel() <- m
			}
			close(s.Channel())
			So(s.Run(), ShouldBeNil)
			return s.Violations()
		}

		Convey("When auditing truthful quotes and conserving volumes", func() {
			measurements := make([]*data.Measurement, 0, 10)

			for idx := range int64(10) {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)

				// Bid (99.0) <= Ask (101.0), midpoint 100.0, relSpread 0.02, feeRate 0.001, hurdle 0.022
				measurement.Write(
					data.NewMetric("best_bid_price", 99.0, data.UnitPrice, data.TimescaleInstantaneous),
					data.NewMetric("best_ask_price", 101.0, data.UnitPrice, data.TimescaleInstantaneous),
					data.NewMetric("relative_spread", 0.02, data.UnitRatio, data.TimescaleInstantaneous),
					data.NewMetric("fee_rate", 0.001, data.UnitRate, data.TimescaleInstantaneous),
					data.NewMetric("friction_hurdle", 0.022, data.UnitRatio, data.TimescaleInstantaneous),
					data.NewMetric("gross_notional", 1000.0, data.UnitVolume, data.TimescaleInstantaneous),
					data.NewMetric("signed_net_fraction", 0.5, data.UnitRatio, data.TimescaleInstantaneous),
				)
				measurement.Finalize()
				measurements = append(measurements, measurement)
			}

			violations := feed(stage, measurements...)
			So(violations, ShouldBeEmpty)
			So(stage.Report(), ShouldBeEmpty)
		})

		Convey("When auditing crossed market order book quotes (bid > ask)", func() {
			measurements := make([]*data.Measurement, 0, 1)
			now := time.Now()
			measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)

			// Best bid 102.0 > Best ask 100.0 (crossed market anomaly)
			measurement.Write(
				data.NewMetric("best_bid_price", 102.0, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("best_ask_price", 100.0, data.UnitPrice, data.TimescaleInstantaneous),
			)
			measurement.Finalize()
			measurements = append(measurements, measurement)

			violations := feed(stage, measurements...)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasCrossed := false
			for _, violation := range violations {
				if violation.Name == "truthfulness.crossed_market" {
					hasCrossed = true
				}
			}
			So(hasCrossed, ShouldBeTrue)
			So(stage.Report(), ShouldContainSubstring, "truthfulness.crossed_market")
		})

		Convey("When auditing relative spread identity breaches", func() {
			measurements := make([]*data.Measurement, 0, 1)
			now := time.Now()
			measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)

			// bid 99, ask 101 -> midpoint 100 -> expected relSpread 0.02, but reported 0.05
			measurement.Write(
				data.NewMetric("best_bid_price", 99.0, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("best_ask_price", 101.0, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("relative_spread", 0.05, data.UnitRatio, data.TimescaleInstantaneous),
			)
			measurement.Finalize()
			measurements = append(measurements, measurement)

			violations := feed(stage, measurements...)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasSpreadViolation := false
			for _, violation := range violations {
				if violation.Name == "truthfulness.identity_divergence" {
					hasSpreadViolation = true
				}
			}
			So(hasSpreadViolation, ShouldBeTrue)
		})

		Convey("When auditing CVD signed_net_fraction out of bounds", func() {
			measurements := make([]*data.Measurement, 0, 1)
			now := time.Now()
			measurement := data.NewMeasurement(1, "BTC/USD", "cvd", now, now)

			// signed_net_fraction 1.5 > 1.0 (impossible fraction)
			measurement.Write(
				data.NewMetric("gross_notional", 50000.0, data.UnitVolume, data.TimescaleInstantaneous),
				data.NewMetric("signed_net_fraction", 1.5, data.UnitRatio, data.TimescaleInstantaneous),
			)
			measurement.Finalize()
			measurements = append(measurements, measurement)

			violations := feed(stage, measurements...)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasCvdViolation := false
			for _, violation := range violations {
				if violation.Name == "truthfulness.volume_divergence" {
					hasCvdViolation = true
				}
			}
			So(hasCvdViolation, ShouldBeTrue)
		})

		Convey("When auditing trade anomalies (non-positive price or quantity)", func() {
			measurements := make([]*data.Measurement, 0, 1)
			now := time.Now()
			measurement := data.NewMeasurement(1, "BTC/USD", "spot:trade", now, now)

			// price <= 0
			measurement.Write(
				data.NewMetric("price", -10.0, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("qty", 1.0, data.UnitVolume, data.TimescaleInstantaneous),
			)
			measurement.Finalize()
			measurements = append(measurements, measurement)

			violations := feed(stage, measurements...)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasTradeViolation := false
			for _, violation := range violations {
				if violation.Name == "truthfulness.trade_anomaly" {
					hasTradeViolation = true
				}
			}
			So(hasTradeViolation, ShouldBeTrue)
		})

		Convey("When auditing non-finite metric values (NaN or Inf)", func() {
			measurements := make([]*data.Measurement, 0, 1)
			now := time.Now()
			measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)

			measurement.Write(
				data.NewMetric("midpoint_log_return", math.NaN(), data.UnitRatio, data.TimescaleInstantaneous),
			)
			measurement.Finalize()
			measurements = append(measurements, measurement)

			violations := feed(stage, measurements...)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasNonFinite := false
			for _, violation := range violations {
				if violation.Name == "truthfulness.non_finite_metric" {
					hasNonFinite = true
				}
			}
			So(hasNonFinite, ShouldBeTrue)
		})

		Convey("When auditing nil measurements", func() {
			nilViolations := feed(stage, nil)
			So(len(nilViolations), ShouldEqual, 1)
			So(nilViolations[0].Name, ShouldEqual, "measurement")
		})
	})
}

