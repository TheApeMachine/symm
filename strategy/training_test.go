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
		measurement := data.NewMeasurement[float64](
			"1", "BTC/USD", "cvd", time.Now().UTC(), time.Time{},
		)
		measurement.PutMetric(data.Metric[float64]{Label: "signed", Raw: 1.5})
		tape.Publish([][]*data.Measurement[float64]{{measurement}})
		tape.Close()
		training := NewTraining(context.Background(), tape)
		envelope := &types.Envelope{}
		So(training.Step(envelope), ShouldEqual, envelope)
		So(training.agents[1].Space().UpdatedLabel, ShouldEqual, "BTC/USD")
		So(training.space.UpdatedLabel, ShouldEqual, "")
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
		So(state.Agents[0].Status, ShouldEqual, "simulated")
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
		envelope := &types.Envelope{
			CVD: data.NewMeasurement[float64]("flow", "BTC/USD", "cvd", time.Now().UTC(), time.Now().UTC()),
		}
		So(envelope.Symbol(), ShouldEqual, "")
		So(training.Step(envelope), ShouldEqual, envelope)
		So(training.space.UpdatedLabel, ShouldEqual, "BTC/USD")
		So(training.Error(), ShouldBeNil)
		So(envelope.StrategyRound, ShouldNotBeNil)
		So(envelope.StrategyRound.Evaluated, ShouldBeTrue)
		So(envelope.StrategyRound.Symbol, ShouldEqual, "BTC/USD")
		So(len(envelope.StrategyRound.Decisions), ShouldEqual, 1)
		So(envelope.StrategyRound.Decisions[0].Action, ShouldEqual, types.ActionNothing)
	})

	Convey("Ring-of-rings rehearsal ingests fragments with random slots and plays with random offsets", t, func() {
		agent := NewAgent(1, false, nil, 64)
		measurementA := data.NewMeasurement[float64]("1", "BTC/USD", "cvd", time.Now().UTC(), time.Time{})
		measurementA.PutMetric(data.Metric[float64]{Label: "signed", Raw: 1.0})

		measurementB := data.NewMeasurement[float64]("2", "BTC/USD", "cvd", time.Now().UTC(), time.Time{})
		measurementB.PutMetric(data.Metric[float64]{Label: "signed", Raw: 2.0})

		fragment := [][]*data.Measurement[float64]{
			{measurementA},
			{measurementB},
		}

		agent.IngestFragment(fragment, 0)
		So(agent.ring.Len(), ShouldEqual, 1)
		So(agent.ring.ChildLen(), ShouldEqual, 2)

		stepped, err := agent.RehearseChild()
		So(err, ShouldBeNil)
		So(stepped, ShouldBeGreaterThanOrEqualTo, 1)
	})
}
