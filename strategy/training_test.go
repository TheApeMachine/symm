package strategy

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/system"
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
		expectedBalance := "200"

		if system.Cfg == nil || system.Cfg.Market == nil || system.Cfg.Market.Balance <= 0 {
			expectedBalance = "10000"
		}
		So(state.Agents[0].Initial, ShouldStartWith, expectedBalance)
		So(state.Agents[0].Cash, ShouldStartWith, expectedBalance)
		So(state.Agents[0].Status, ShouldEqual, "simulated")
		So(state.Agents[1].Status, ShouldEqual, "learning")
		So(state.Rehearsal, ShouldNotBeNil)
		So(state.Rehearsal.Workers, ShouldEqual, 8)
		So(len(state.Rehearsal.Tracks), ShouldEqual, 8)
		So(len(state.Recognition.Learners), ShouldEqual, 8)
	})

	Convey("Live step with empty symbol falls back to measurement label without unknown context error", t, func() {
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
	})
}
