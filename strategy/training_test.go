package strategy

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

func TestTrainingStep(t *testing.T) {
	Convey("An empty pipeline returns the envelope without panicking", t, func() {
		tape := NewTape()
		tape.Close()
		training := NewTraining(context.Background(), tape)
		envelope := &types.Envelope{}
		So(training.Step(envelope), ShouldEqual, envelope)
		So(envelope.Learning, ShouldEqual, training)
		So(training.Error(), ShouldBeNil)
	})

	Convey("A tape frame flows through the nomagique composition", t, func() {
		tape := NewTape()
		measurement := data.NewMeasurement[float64]("cvd", nil)
		measurement.Label, measurement.At, measurement.From = "BTC/USD", time.Now().UTC(), time.Time{}
		measurement.Metrics["signed"] = data.Metric[float64]{Label: "signed", Raw: 1.5}
		measurement2 := data.NewMeasurement[float64]("cvd", nil)
		measurement2.Label, measurement2.At, measurement2.From = "BTC/USD", time.Now().UTC(), time.Time{}
		measurement2.Metrics["signed"] = data.Metric[float64]{Label: "signed", Raw: 1.8}
		tape.Publish(types.ReplayFragment{
			Frames:      [][]*data.Measurement[float64]{{measurement}, {measurement2}},
			Symbol:      "BTC/USD",
			AnchorIndex: 1,
		})
		tape.Close()
		training := NewTraining(context.Background(), tape)
		envelope := &types.Envelope{}
		So(training.Step(envelope), ShouldEqual, envelope)
		So(spaceState(training.agents[1].Space()).Updated, ShouldEqual, "BTC/USD")
		So(spaceState(training.space).Updated, ShouldEqual, "")
		So(training.Error(), ShouldBeNil)
	})

	Convey("Multi-agent population instantiates 8 traders by default and serializes all learners", t, func() {
		tape := NewTape()
		tape.Close()
		training := NewTraining(context.Background(), tape)
		So(len(training.agents), ShouldEqual, 8)
		So(training.agents[0].IsLive(), ShouldBeTrue)
		So(training.agents[1].IsLive(), ShouldBeFalse)

		state := training.State().state
		So(state, ShouldNotBeNil)
		So(len(state.Agents), ShouldEqual, 8)
		So(state.Agents[0].Initial, ShouldStartWith, "200")
		So(state.Agents[0].Cash, ShouldStartWith, "200")
		So(state.Agents[0].Status, ShouldEqual, "paper")
		So(state.Agents[1].Status, ShouldEqual, "learning")
		So(state.Rehearsal, ShouldNotBeNil)
		So(state.Rehearsal.Workers, ShouldEqual, 8)
		So(len(state.Rehearsal.Tracks), ShouldEqual, 8)
		So(len(state.Recognition.Learners), ShouldEqual, 8)
	})

	Convey("Live step with empty symbol falls back to measurement label and emits decision frame", t, func() {
		tape := NewTape()
		tape.Close()
		training := NewTraining(context.Background(), tape)
		cvd := data.NewMeasurement[float64]("cvd", map[string]data.Metric[float64]{})
		cvd.Label, cvd.At, cvd.From = "BTC/USD", time.Now().UTC(), time.Now().UTC()
		envelope := &types.Envelope{
			CVD: cvd,
		}
		So(envelope.Symbol(), ShouldEqual, "")
		So(training.Step(envelope), ShouldEqual, envelope)
		So(spaceState(training.space).Updated, ShouldEqual, "BTC/USD")
		So(training.Error(), ShouldBeNil)
		So(envelope.StrategyRound, ShouldNotBeNil)
		So(envelope.StrategyRound.Evaluated, ShouldBeTrue)
		So(envelope.StrategyRound.Symbol, ShouldEqual, "BTC/USD")
		So(len(envelope.StrategyRound.Decisions), ShouldEqual, 1)
		So(envelope.StrategyRound.Decisions[0].Action, ShouldEqual, types.ActionNothing)
	})

	Convey("Ring-of-rings rehearsal ingests fragments with random slots and plays with random offsets", t, func() {
		agent := NewAgent(1, false, nil, 64)
		measurementA := data.NewMeasurement[float64]("cvd", nil)
		measurementA.Label, measurementA.At, measurementA.From = "BTC/USD", time.Now().UTC(), time.Time{}
		measurementA.Metrics["signed"] = data.Metric[float64]{Label: "signed", Raw: 1.0}

		measurementB := data.NewMeasurement[float64]("cvd", nil)
		measurementB.Label, measurementB.At, measurementB.From = "BTC/USD", time.Now().UTC(), time.Time{}
		measurementB.Metrics["signed"] = data.Metric[float64]{Label: "signed", Raw: 2.0}

		fragment := types.ReplayFragment{
			Frames: [][]*data.Measurement[float64]{
				{measurementA},
				{measurementB},
			},
			Symbol:      "BTC/USD",
			AnchorIndex: 1,
		}

		agent.IngestReplay(fragment, 0)
		So(ringLen[[]*data.Measurement[float64]](agent.ring), ShouldEqual, 1)
		So(ringChildLen[[]*data.Measurement[float64]](agent.ring), ShouldEqual, 2)

		stepped, err := agent.RehearseChild()
		So(err, ShouldBeNil)
		So(stepped, ShouldBeGreaterThanOrEqualTo, 1)
	})
}
