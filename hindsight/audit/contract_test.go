package audit

import (
	"context"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestContractStage(t *testing.T) {
	Convey("Given a ContractStage instance", t, func() {
		ctx := context.Background()
		stage := NewContractStage(ctx)

		feed := func(s *ContractStage, ms ...*data.Measurement) []*Violation {
			for _, m := range ms {
				s.Channel() <- m
			}
			close(s.Channel())
			So(s.Run(), ShouldBeNil)
			return s.Violations()
		}

		Convey("When auditing valid measurements", func() {
			now := time.Now()
			measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)
			metric := data.NewMetric("test_metric", 100.0, data.UnitPrice, data.TimescaleInstantaneous)
			measurement.Write(metric)

			violations := feed(stage, measurement)
			So(violations, ShouldBeEmpty)
			So(stage.Report(), ShouldBeEmpty)
		})

		Convey("When auditing nil measurements", func() {
			violations := feed(stage, nil)
			So(len(violations), ShouldEqual, 1)
			So(violations[0].Name, ShouldEqual, "measurement")
		})

		Convey("When auditing price metrics with negative values", func() {
			now := time.Now()
			measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)
			metric := data.NewMetric("test_price", -50.0, data.UnitPrice, data.TimescaleInstantaneous)
			measurement.Write(metric)

			violations := feed(stage, measurement)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasViolation := false
			for _, violation := range violations {
				if violation.Description == "Price must be strictly positive (> 0)" {
					hasViolation = true
				}
			}
			So(hasViolation, ShouldBeTrue)
		})

		Convey("When auditing probability metrics with out-of-bound values", func() {
			now := time.Now()
			measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)
			metric := data.NewMetric("test_prob", 1.5, data.UnitProbability, data.TimescaleInstantaneous)
			measurement.Write(metric)

			violations := feed(stage, measurement)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasViolation := false
			for _, violation := range violations {
				if violation.Description == "Probability must be bounded in [0, 1]" {
					hasViolation = true
				}
			}
			So(hasViolation, ShouldBeTrue)
		})

		Convey("When auditing physical quantities with negative values", func() {
			now := time.Now()
			measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)
			metric := data.NewMetric("test_qty", -1.0, data.UnitQuantity, data.TimescaleInstantaneous)
			measurement.Write(metric)

			violations := feed(stage, measurement)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasViolation := false
			for _, violation := range violations {
				if violation.Description == "Physical quantity cannot be negative" {
					hasViolation = true
				}
			}
			So(hasViolation, ShouldBeTrue)
		})

		Convey("When auditing correlations with out-of-bound values", func() {
			now := time.Now()
			measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)
			metric := data.NewMetric("test_corr", -1.5, data.UnitCorrelation, data.TimescaleInstantaneous)
			measurement.Write(metric)

			violations := feed(stage, measurement)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasViolation := false
			for _, violation := range violations {
				if violation.Description == "Correlation must be bounded in [-1, 1]" {
					hasViolation = true
				}
			}
			So(hasViolation, ShouldBeTrue)
		})

		Convey("When validating bounds directly", func() {
			now := time.Now()
			measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)
			metric := data.NewMetric("test_snm", 0.0, data.UnitPrice, data.TimescaleInstantaneous)

			stage.validateBounds(2.0, metric, measurement, -1.0, 1.0, "SNM bound breach")
			So(len(stage.violations), ShouldEqual, 1)
			So(stage.violations[0].Description, ShouldEqual, "SNM bound breach")

			stage.validateBounds(math.NaN(), metric, measurement, -1.0, 1.0, "NaN bound breach")
		})
	})
}
