package audit

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
)

func TestVitalityStage(t *testing.T) {
	Convey("Given a VitalityStage instance", t, func() {
		ctx := context.Background()
		stage := NewVitalityStage(ctx)

		feed := func(s *VitalityStage, ms ...*data.Value) []*Violation {
			for _, m := range ms {
				s.Channel() <- m
			}
			close(s.Channel())
			So(s.Run(), ShouldBeNil)
			return s.Violations()
		}

		Convey("When auditing healthy varying metrics", func() {
			measurements := make([]*data.Value, 0, 15)

			for idx := range int64(15) {
				val := data.NewValue("test", "test", "BTC/USD")
				data.AppendVector(val, []float64{float64(idx+1) * 2.5}, "healthy_var")
				measurements = append(measurements, val)
			}

			violations := feed(stage, measurements...)
			So(violations, ShouldBeEmpty)
			So(stage.Report(), ShouldBeEmpty)
		})

		Convey("When auditing dead/constant metrics with zero variance", func() {
			measurements := make([]*data.Value, 0, 15)

			for range int64(15) {
				val := data.NewValue("test", "test", "BTC/USD")
				data.AppendVector(val, []float64{42.0}, "dead_const")
				measurements = append(measurements, val)
			}

			violations := feed(stage, measurements...)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasDeadViolation := false
			for _, violation := range violations {
				if violation.Name == "vitality.dead_metric" {
					hasDeadViolation = true
				}
			}
			So(hasDeadViolation, ShouldBeTrue)
			So(stage.Report(), ShouldContainSubstring, "vitality.dead_metric")
		})

		Convey("When auditing stagnant/cold-start metrics with high zero fractions", func() {
			measurements := make([]*data.Value, 0, 15)

			for idx := range int64(15) {
				val := data.NewValue("test", "test", "BTC/USD")
				v := 0.0
				if idx == 14 {
					v = 1.0 // Only 1 non-zero out of 15 (93.3% zero)
				}
				data.AppendVector(val, []float64{v}, "stagnant_sig")
				measurements = append(measurements, val)
			}

			violations := feed(stage, measurements...)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasStagnant := false
			for _, violation := range violations {
				if violation.Name == "vitality.stagnant_metric" {
					hasStagnant = true
				}
			}
			So(hasStagnant, ShouldBeTrue)
		})

		Convey("When auditing nil measurements", func() {
			nilViolations := feed(stage, nil)
			// A nil-only tape is a nil violation and, carrying nothing, insufficient evidence.
			So(hasViolation(nilViolations, "measurement"), ShouldBeTrue)
			So(hasInsufficient(nilViolations), ShouldBeTrue)
		})
	})
}
