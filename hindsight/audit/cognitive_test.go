package audit

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
)

func TestCognitiveStage(t *testing.T) {
	Convey("Given a CognitiveStage instance", t, func() {
		ctx := context.Background()
		stage := NewCognitiveStage(ctx)

		feed := func(s *CognitiveStage, ms []*data.Value) []*Violation {
			for _, m := range ms {
				s.Channel() <- m
			}
			close(s.Channel())
			So(s.Run(), ShouldBeNil)
			return s.Violations()
		}

		Convey("When auditing decisive cognitive phases with valid confidence", func() {
			measurements := make([]*data.Value, 0, 25)

			for idx := range int64(25) {
				measurement := data.NewValue("test", "test", "BTC/USD")

				actionVal := 0.0 // default wait
				if idx%5 == 0 {
					actionVal = 1.0 // enter
				}

				data.AppendVector(measurement, []float64{actionVal, 0.75}, "action", "confidence")
				measurements = append(measurements, measurement)
			}

			violations := feed(stage, measurements)
			So(violations, ShouldBeEmpty)
			So(stage.Report(), ShouldBeEmpty)
		})

		Convey("When auditing complete abstention (100% wait actions)", func() {
			measurements := make([]*data.Value, 0, 25)

			for range int64(25) {
				measurement := data.NewValue("test", "test", "BTC/USD")

				// No action metric emitted -> defaults to wait
				data.AppendVector(measurement, []float64{0.0}, "confidence")
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
			measurements := make([]*data.Value, 0, 25)

			for range int64(25) {
				measurement := data.NewValue("test", "test", "BTC/USD")

				data.AppendVector(measurement, []float64{1.0, 0.0}, "action", "confidence")
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
			nilViolations := feed(stage, []*data.Value{nil})
			// A nil-only tape is a nil violation and, carrying nothing, insufficient evidence.
			So(hasViolation(nilViolations, "measurement"), ShouldBeTrue)
			So(hasInsufficient(nilViolations), ShouldBeTrue)
		})
	})
}
