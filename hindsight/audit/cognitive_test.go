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

		Convey("Published policies report the gate they were trained under", func() {
			enter := policyFixture(1, "enter", "R01/R04", 0.01)
			exit := policyFixture(2, "exit", "R02/R03", 0.01)
			So(feed(stage, []*data.Value{enter, exit}), ShouldBeEmpty)

			gate := stage.Data().(map[string]any)["gate"].(map[string]any)
			So(gate["enabled"], ShouldEqual, true)
			So(gate["policy_epoch"], ShouldEqual, "1792187257188073000")
		})

		Convey("A policy without a gate tag is a violation", func() {
			policy := &data.Value{Origin: data.PolicyOrigin, Role: "enter", Scope: data.PolicyScope, Epoch: realizationEpoch, Tick: 1}
			policy.FormatPrefix()
			So(data.AppendPolicy(policy, "R01/R04", []float64{0.01, 30, 4, 0.01, 0.75}), ShouldBeNil)

			violations := feed(stage, []*data.Value{policy})
			So(violations, ShouldHaveLength, 1)
			So(violations[0].Name, ShouldEqual, "cognitive.gate")
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
