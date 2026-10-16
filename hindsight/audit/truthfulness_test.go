package audit

import (
	"context"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
)

func TestTruthfulnessStage(t *testing.T) {
	Convey("Given a TruthfulnessStage instance", t, func() {
		ctx := context.Background()
		stage := NewTruthfulnessStage(ctx)

		feed := func(s *TruthfulnessStage, ms ...*data.Value) []*Violation {
			for _, m := range ms {
				s.Channel() <- m
			}
			close(s.Channel())
			So(s.Run(), ShouldBeNil)
			return s.Violations()
		}

		Convey("When auditing truthful quotes and conserving volumes", func() {
			measurements := make([]*data.Value, 0, 10)

			for idx := range int64(10) {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				val := data.NewValue("test", "tick", "BTC/USD")
				val.Timestamp = uint64(now.UnixNano())
				val.Tick = uint64(idx + 1)

				data.AppendVector(val, []float64{99.0}, "best_price:bid")
				data.AppendVector(val, []float64{101.0}, "best_price:ask")
				data.AppendVector(val, []float64{0.02}, "relative_spread")
				data.AppendVector(val, []float64{0.001}, "fee_rate")
				data.AppendVector(val, []float64{0.022}, "friction_hurdle")
				data.AppendVector(val, []float64{1000.0}, "gross_notional")
				data.AppendVector(val, []float64{0.5}, "signed_net_fraction")

				measurements = append(measurements, val)
			}

			violations := feed(stage, measurements...)
			So(violations, ShouldBeEmpty)
			So(stage.Report(), ShouldBeEmpty)
		})

		Convey("When auditing crossed market order book quotes (bid > ask)", func() {
			val := data.NewValue("test", "tick", "BTC/USD")
			data.AppendVector(val, []float64{102.0}, "best_price:bid")
			data.AppendVector(val, []float64{100.0}, "best_price:ask")

			violations := feed(stage, val)
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
			val := data.NewValue("test", "tick", "BTC/USD")
			data.AppendVector(val, []float64{99.0}, "best_price:bid")
			data.AppendVector(val, []float64{101.0}, "best_price:ask")
			data.AppendVector(val, []float64{0.05}, "relative_spread")

			violations := feed(stage, val)
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
			val := data.NewValue("cvd", "tick", "BTC/USD")
			data.AppendVector(val, []float64{50000.0}, "gross_notional")
			data.AppendVector(val, []float64{1.5}, "signed_net_fraction")

			violations := feed(stage, val)
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
			val := data.NewValue("spot", "trade", "BTC/USD")
			data.AppendVector(val, []float64{-10.0}, "price")
			data.AppendVector(val, []float64{1.0}, "qty")

			violations := feed(stage, val)
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
			val := data.NewValue("test", "tick", "BTC/USD")
			data.AppendVector(val, []float64{math.NaN()}, "midpoint_log_return")

			violations := feed(stage, val)
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
			// A nil-only tape is a nil violation and, counting nothing, insufficient evidence.
			So(hasViolation(nilViolations, "measurement"), ShouldBeTrue)
			So(hasInsufficient(nilViolations), ShouldBeTrue)
		})
	})
}
