package audit

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
)

func TestContractStage(t *testing.T) {
	Convey("Given a ContractStage instance", t, func() {
		ctx := context.Background()
		stage := NewContractStage(ctx)

		feed := func(s *ContractStage, ms ...*data.Value) []*Violation {
			for _, m := range ms {
				s.Channel() <- m
			}
			close(s.Channel())
			So(s.Run(), ShouldBeNil)
			return s.Violations()
		}

		Convey("When auditing valid measurements", func() {
			val := data.NewValue("test", "test", "BTC/USD")
			data.AppendVector(val, []float64{100.0}, "test_metric")

			violations := feed(stage, val)
			So(violations, ShouldBeEmpty)
			So(stage.Report(), ShouldBeEmpty)
		})

		Convey("When auditing nil measurements", func() {
			violations := feed(stage, nil)
			So(len(violations), ShouldEqual, 1)
			So(violations[0].Name, ShouldEqual, "measurement")
		})

		Convey("When auditing price metrics with negative values", func() {
			val := data.NewValue("test", "test", "BTC/USD")
			data.AppendVector(val, []float64{-50.0}, "test_price")

			violations := feed(stage, val)
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
			val := data.NewValue("test", "test", "BTC/USD")
			data.AppendVector(val, []float64{1.5}, "test_prob")

			violations := feed(stage, val)
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
			val := data.NewValue("test", "test", "BTC/USD")
			data.AppendVector(val, []float64{-1.0}, "test_qty")

			violations := feed(stage, val)
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
			val := data.NewValue("test", "test", "BTC/USD")
			data.AppendVector(val, []float64{-1.5}, "test_corr")

			violations := feed(stage, val)
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
			val := data.NewValue("test", "test", "BTC/USD")

			stage.validateBounds(2.0, "test_snm", val, -1.0, 1.0, "SNM bound breach")
			So(len(stage.violations), ShouldEqual, 1)
			So(stage.violations[0].Description, ShouldEqual, "SNM bound breach")

			stage.validateBounds(math.NaN(), "test_snm", val, -1.0, 1.0, "NaN bound breach")
			So(len(stage.violations), ShouldEqual, 2)
		})
	})
}
