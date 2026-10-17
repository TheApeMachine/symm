package audit

import (
	"context"
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
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

		Convey("A stored sequence without a gate tag is counted", func() {
			policy := data.NewValue("policy", "enter", "BTC/USD")
			data.AppendVector(policy, []float64{0.01, 0, 1, 0.01, 1}, "gain", "hold_seconds", "visits", "mean_reward", "confidence", "path:R01/R05")

			violations := feed(stage, []*data.Value{policy})
			So(violations, ShouldBeEmpty)

			fields := stage.Data().(map[string]any)
			So(fields["enter_count"], ShouldEqual, 1)
			So(fields["total_phases"], ShouldEqual, 1)
			So(stage.phases[0].Path, ShouldEqual, "R01/R05")
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

func TestLiveCurrentSequences(t *testing.T) {
	if os.Getenv("AUDIT_LIVE") != "1" {
		t.Skip()
	}

	ctx := context.Background()
	client := store.NewClient(ctx, system.Cfg.Storage.S3.Bucket)

	if client == nil {
		t.Fatal("store client")
	}

	stage := NewCognitiveStage(ctx)

	go func() {
		for value, err := range currentSequences(client) {
			if err != nil {
				t.Error(err)
				continue
			}

			if value != nil && value.Origin == data.PolicyOrigin {
				stage.Channel() <- value
			}
		}

		close(stage.Channel())
	}()

	if err := stage.Run(); err != nil {
		t.Fatal(err)
	}

	fields := stage.Data().(map[string]any)
	t.Logf("sequences %v enter %v exit %v noop %v", fields["total_phases"], fields["enter_count"], fields["exit_count"], fields["noop_count"])

	if fields["total_phases"] == 0 {
		t.Fatal("policy/current has no sequences")
	}
}
