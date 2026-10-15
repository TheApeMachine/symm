package audit

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestCognitiveStage(t *testing.T) {
	Convey("Given a CognitiveStage instance", t, func() {
		ctx := context.Background()
		stage := NewCognitiveStage(ctx)

		feed := func(s *CognitiveStage, ms []*data.Measurement) []*Violation {
			for _, m := range ms {
				s.Channel() <- m
			}
			close(s.Channel())
			So(s.Run(), ShouldBeNil)
			return s.Violations()
		}

		Convey("When auditing decisive cognitive phases with valid confidence", func() {
			measurements := make([]*data.Measurement, 0, 25)

			for idx := range int64(25) {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)

				actionVal := 0.0 // default wait
				if idx%5 == 0 {
					actionVal = 1.0 // enter
				}

				measurement.Write(
					data.NewMetric("action", actionVal, data.UnitDimensionless, data.TimescaleInstantaneous),
					data.NewMetric("confidence", 0.75, data.UnitProbability, data.TimescaleInstantaneous),
				)
				measurement.Finalize()
				measurements = append(measurements, measurement)
			}

			violations := feed(stage, measurements)
			So(violations, ShouldBeEmpty)
			So(stage.Report(), ShouldBeEmpty)
		})

		Convey("When auditing complete abstention (100% wait actions)", func() {
			measurements := make([]*data.Measurement, 0, 25)

			for idx := range int64(25) {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)

				// No action metric emitted -> defaults to wait
				measurement.Write(
					data.NewMetric("confidence", 0.0, data.UnitProbability, data.TimescaleInstantaneous),
				)
				measurement.Finalize()
				measurements = append(measurements, measurement)
			}

			violations := feed(stage, measurements)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasAbstention := false
			for _, violation := range violations {
				if violation.Name == "cognitive.complete_abstention" {
					hasAbstention = true
				}
			}
			So(hasAbstention, ShouldBeTrue)
			So(stage.Report(), ShouldContainSubstring, "cognitive.complete_abstention")
		})

		Convey("When auditing active actions with zero confidence", func() {
			measurements := make([]*data.Measurement, 0, 25)

			for idx := range int64(25) {
				now := time.Now().Add(time.Duration(idx) * time.Millisecond)
				measurement := data.NewMeasurement(1, "BTC/USD", "test", now, now)

				measurement.Write(
					data.NewMetric("action", 1.0, data.UnitDimensionless, data.TimescaleInstantaneous),
					data.NewMetric("confidence", 0.0, data.UnitProbability, data.TimescaleInstantaneous),
				)
				measurement.Finalize()
				measurements = append(measurements, measurement)
			}

			violations := feed(stage, measurements)
			So(len(violations), ShouldBeGreaterThanOrEqualTo, 1)

			hasZeroConf := false
			for _, violation := range violations {
				if violation.Name == "cognitive.zero_confidence" {
					hasZeroConf = true
				}
			}
			So(hasZeroConf, ShouldBeTrue)
		})

		Convey("When auditing nil measurements", func() {
			nilViolations := feed(stage, []*data.Measurement{nil})
			So(len(nilViolations), ShouldEqual, 1)
			So(nilViolations[0].Name, ShouldEqual, "measurement")
		})
	})
}
