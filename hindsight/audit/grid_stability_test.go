package audit

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestStabilityStage(t *testing.T) {
	Convey("Given a StabilityStage instance", t, func() {
		ctx := context.Background()
		stage := NewStabilityStage(ctx)

		feed := func(s *StabilityStage, ms ...*data.Measurement) []*Violation {
			for _, m := range ms {
				s.Channel() <- m
			}
			close(s.Channel())
			_ = s.Run()
			return s.Violations()
		}

		Convey("When auditing stationary grid activations across halves", func() {
			measurements := make([]*data.Measurement, 0, 30)

			// Both halves alternate between token_a and token_b evenly
			for idx := range int64(30) {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)

				label := "token_a"
				if idx%2 == 1 {
					label = "token_b"
				}

				measurement.Write(data.NewMetric(label, 1.0, data.UnitDimensionless, data.TimescaleInstantaneous))
				measurement.Finalize()
				measurements = append(measurements, measurement)
			}

			violations := feed(stage, measurements...)
			So(violations, ShouldBeEmpty)
			So(stage.Report(), ShouldBeEmpty)
		})

		Convey("When auditing severe non-stationary regime shift between halves", func() {
			measurements := make([]*data.Measurement, 0, 30)

			// First half: 15 frames of token_a
			for idx := range int64(15) {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)
				measurement.Write(data.NewMetric("token_a", 1.0, data.UnitDimensionless, data.TimescaleInstantaneous))
				measurement.Finalize()
				measurements = append(measurements, measurement)
			}

			// Second half: 15 frames of token_b (completely disjoint distributions, JSD = 1.0 bit)
			for idx := int64(15); idx < 30; idx++ {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)
				measurement.Write(data.NewMetric("token_b", 1.0, data.UnitDimensionless, data.TimescaleInstantaneous))
				measurement.Finalize()
				measurements = append(measurements, measurement)
			}

			violations := feed(stage, measurements...)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasNonStationary := false
			for _, violation := range violations {
				if violation.Name == "grid.non_stationary" {
					hasNonStationary = true
				}
			}
			So(hasNonStationary, ShouldBeTrue)
			So(stage.Report(), ShouldContainSubstring, "grid.non_stationary")
		})

		Convey("When auditing incomplete grid observations", func() {
			measurements := make([]*data.Measurement, 0, 25)

			// 25 measurements without any metrics (0% completeness)
			for idx := range int64(25) {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)
				measurement.Finalize()
				measurements = append(measurements, measurement)
			}

			violations := feed(stage, measurements...)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasIncomplete := false
			for _, violation := range violations {
				if violation.Name == "grid.incomplete" {
					hasIncomplete = true
				}
			}
			So(hasIncomplete, ShouldBeTrue)
		})

		Convey("When auditing nil measurements", func() {
			nilViolations := feed(stage, nil)
			So(len(nilViolations), ShouldEqual, 1)
			So(nilViolations[0].Name, ShouldEqual, "measurement")
		})
	})
}
