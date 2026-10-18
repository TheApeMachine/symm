package audit

import (
	"context"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
)

const realizationEpoch = uint64(1792187257188073000)

/*
policyFixture is a published policy for path with gain predicted.
*/
func policyFixture(seq uint64, action, path string, predicted float64) *data.Value {
	policy := data.NewValue("policy", action, "all")
	policy.Epoch = realizationEpoch
	policy.Tick = seq
	policy.Append([]float64{predicted, 30, 4, predicted, 0.75}, "gain", "hold_seconds", "visits", "mean_reward", "confidence", "path:"+path)
	policy.Append([]float64{
		1, fixtureGate.SurpriseZ, fixtureGate.Warmup.FastRate, fixtureGate.Warmup.SlowRate, fixtureGate.LearningRate,
	}, "enabled", "surprise_z", "warmup_fast_rate", "warmup_slow_rate", "learning_rate", "gate", "warmup:"+fixtureGate.Warmup.Rule, fmt.Sprintf("ticks:%d", fixtureGate.TickEpoch))
	return policy
}

var fixtureGate = policyGate{
	TickEpoch:    realizationEpoch - 1,
	Enabled:      true,
	SurpriseZ:    1,
	Warmup:       policyWarmup{Rule: "settled", FastRate: 0.15, SlowRate: 0.01},
	LearningRate: 0.01,
}

func paperFixture(seq uint64, action, path string, policyEpoch uint64, netReturn float64) *data.Value {
	record := data.NewValue("paper", action, "BTC/USD")
	record.Epoch = realizationEpoch + 99
	record.Tick = seq
	vector := make([]float64, 15)
	vector[len(vector)-1] = netReturn
	record.Append(vector,
		"depth", "intended_price", "fill_price", "decided_at", "filled_at",
		"latency_seconds", "slippage", "qty", "entry_price", "exit_price",
		"entered_at", "exited_at", "fees", "pnl", "net_return",
		"path:"+path, fmt.Sprintf("policy_epoch:%d", policyEpoch),
	)
	return record
}

func TestRealizationStage(t *testing.T) {
	Convey("Given a RealizationStage", t, func() {
		stage := NewRealizationStage(context.Background())

		feed := func(values ...*data.Value) []*Violation {
			go func() {
				for _, value := range values {
					stage.Channel() <- value
				}

				close(stage.Channel())
			}()

			So(stage.Run(), ShouldBeNil)
			return stage.Violations()
		}

		paths := func(predicted func(int) float64, realized func(int) []float64) []*data.Value {
			values := make([]*data.Value, 0)
			seq := uint64(0)

			for idx := range 8 {
				path := fmt.Sprintf("R%02d/R%02d", idx, idx+1)
				values = append(values, policyFixture(uint64(idx+1), "enter", path, predicted(idx)))

				for _, sample := range realized(idx) {
					seq++
					values = append(values, paperFixture(seq, "enter", path, realizationEpoch, sample))
				}
			}

			return values
		}

		Convey("It passes when realized edge tracks predicted edge across paths", func() {
			violations := feed(paths(
				func(idx int) float64 { return 0.01*float64(idx-3) + 0.005 },
				func(idx int) []float64 { return []float64{0.01*float64(idx-3) + 0.011, 0.01*float64(idx-3) + 0.013} },
			)...)

			So(violations, ShouldBeEmpty)

			fields := stage.Data().(map[string]any)
			So(fields["passed"], ShouldEqual, true)
			So(fields["gate"], ShouldResemble, map[string]any{
				"policy_epoch": fmt.Sprint(realizationEpoch), "tick_epoch": fmt.Sprint(realizationEpoch - 1),
				"enabled": true, "surprise_z": 1.0, "warmup": "settled: error average at 0.15 no longer below its average at 0.01", "learning_rate": 0.01,
			})
			So(fields["paper_records"], ShouldEqual, 16)
			So(fields["traded_paths"], ShouldEqual, 8)
			So(fields["correlation"].(float64), ShouldAlmostEqual, 1, 1e-9)
			// R02/R03 was predicted to lose 0.005 and realized +0.002.
			So(fields["sign_agreement"].(float64), ShouldAlmostEqual, 7.0/8, 1e-9)
			So(fields["mean_realized_minus_predicted"].(float64), ShouldAlmostEqual, 0.007, 1e-9)
			So(stage.Report(), ShouldContainSubstring, "R00/R01: 2 samples")
		})

		Convey("It fails when realized edge runs against predicted edge", func() {
			violations := feed(paths(
				func(idx int) float64 { return 0.01 * float64(idx) },
				func(idx int) []float64 { return []float64{-0.01 * float64(idx)} },
			)...)

			So(hasViolation(violations, "realization.unpredictive"), ShouldBeTrue)
			So(stage.Data().(map[string]any)["passed"], ShouldEqual, false)
		})

		Convey("With no paper records it fails, saying there is no paper data", func() {
			violations := feed(policyFixture(1, "enter", "R01/R04", 0.02))

			So(hasInsufficient(violations), ShouldBeTrue)
			So(stage.Report(), ShouldContainSubstring, "no paper data")
			So(stage.Data().(map[string]any)["passed"], ShouldEqual, false)
		})

		Convey("A record that traded a path the policy epoch does not store is a violation", func() {
			violations := feed(
				policyFixture(1, "enter", "R01/R04", 0.02),
				paperFixture(1, "enter", "R09/R09", realizationEpoch, 0.01),
			)

			So(hasViolation(violations, "realization.unknown_path"), ShouldBeTrue)
		})

		Convey("Records only from other policy epochs are insufficient, not a pass", func() {
			violations := feed(
				policyFixture(1, "enter", "R01/R04", 0.02),
				paperFixture(1, "enter", "R01/R04", realizationEpoch-1, 0.01),
			)

			So(hasInsufficient(violations), ShouldBeTrue)
			So(stage.Data().(map[string]any)["other_epoch_records"], ShouldEqual, 1)
		})

		Convey("Paths whose edges do not vary cannot be correlated: insufficient", func() {
			violations := feed(paths(
				func(int) float64 { return 0.02 },
				func(int) []float64 { return []float64{0.01} },
			)...)

			So(hasInsufficient(violations), ShouldBeTrue)
		})

		Convey("A malformed record is a violation", func() {
			broken := &data.Value{Origin: "paper", Role: "enter", Scope: "BTC/USD", Epoch: 1, Tick: 1}

			violations := feed(policyFixture(1, "enter", "R01/R04", 0.02), broken)
			So(hasViolation(violations, "realization.malformed"), ShouldBeTrue)
		})
	})
}
