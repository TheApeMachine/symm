package audit

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestVitalityStage(t *testing.T) {
	Convey("Given a VitalityStage instance", t, func() {
		ctx := context.Background()
		stage := NewVitalityStage(ctx)

		feed := func(s *VitalityStage, ms ...*data.Measurement) []*Violation {
			for _, m := range ms {
				s.Channel() <- m
			}
			close(s.Channel())
			So(s.Run(), ShouldBeNil)
			return s.Violations()
		}

		Convey("When auditing healthy varying metrics", func() {
			measurements := make([]*data.Measurement, 0, 15)

			for idx := range int64(15) {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)
				val := float64(idx+1) * 2.5
				measurement.Write(data.NewMetric("healthy_var", val, data.UnitPrice, data.TimescaleInstantaneous))
				measurements = append(measurements, measurement)
			}

			violations := feed(stage, measurements...)
			So(violations, ShouldBeEmpty)
			So(stage.Report(), ShouldBeEmpty)
		})

		Convey("When auditing dead/constant metrics with zero variance", func() {
			measurements := make([]*data.Measurement, 0, 15)

			for idx := range int64(15) {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)
				measurement.Write(data.NewMetric("dead_const", 42.0, data.UnitPrice, data.TimescaleInstantaneous))
				measurements = append(measurements, measurement)
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
			measurements := make([]*data.Measurement, 0, 15)

			for idx := range int64(15) {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)
				val := 0.0

				if idx == 14 {
					val = 1.0 // Only 1 non-zero out of 15 (93.3% zero)
				}

				measurement.Write(data.NewMetric("stagnant_sig", val, data.UnitDimensionless, data.TimescaleInstantaneous))
				measurements = append(measurements, measurement)
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

		Convey("When auditing nil measurements and nil metric entries", func() {
			nilViolations := feed(stage, nil)
			So(len(nilViolations), ShouldEqual, 1)
			So(nilViolations[0].Name, ShouldEqual, "measurement")

			now := time.Now()
			measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)
			measurement.Write(&data.Metric{})
			entryStage := NewVitalityStage(ctx)
			violations := feed(entryStage, measurement)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)
		})
	})
}
