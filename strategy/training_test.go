package strategy

import (
	"iter"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/tests/market"
)

func trainingTape(frames []*data.Measurement[float64]) iter.Seq2[*data.Measurement[float64], error] {
	return func(yield func(*data.Measurement[float64], error) bool) {
		for _, frame := range frames {
			if !yield(frame, nil) {
				return
			}
		}
	}
}

func TestTrainingRegister(t *testing.T) {
	Convey("Training owns its telemetry and declares all upstream producers", t, func() {
		training := NewTraining(t.Context(), 1, market.TrainingPrice(t.Context()))
		So(training.Register(), ShouldEqual, training.Register())
		So(training.Register().Metadata["peer-interest"], ShouldEqual, "*")
		_, found := training.Register().Metrics["input_count"]
		So(found, ShouldBeTrue)
	})
}

func TestTrainingStep(t *testing.T) {
	Convey("Untrained live inference remains inert and publishes the actual map", t, func() {
		training := NewTraining(t.Context(), 1, market.TrainingPrice(t.Context()))
		training.Transition(runtime.READY)

		for _, frame := range market.ImpulseTape("BTC/USD", 4) {
			output := training.Step(frame)
			So(output.Err, ShouldBeNil)
			_, selected := output.Metrics["action"]
			So(selected, ShouldBeFalse)
			So(output.Result, ShouldBeNil)
			So(training.Grid.Markets["BTC/USD"].Sequence, ShouldEqual, frame.SeqIdx)
			So(output.Metrics["input_count"].Raw, ShouldEqual, 3)
		}
	})

	Convey("An inactive pipeline does not mutate its input", t, func() {
		node := &Training{System: runtime.NewSystem(t.Context(), "readiness-test")}
		measurement := &data.Measurement[float64]{Label: "BTC/USD", SeqIdx: 7}

		for _, stage := range []runtime.Stage{runtime.INIT, runtime.WAITING, runtime.ERROR, runtime.FATAL} {
			node.Transition(stage)
			So(node.Step(measurement), ShouldEqual, measurement)
			So(measurement.SeqIdx, ShouldEqual, 7)
		}
	})
}

func BenchmarkTrainingStep(b *testing.B) {
	training := NewTraining(b.Context(), 1, market.TrainingPrice(b.Context()))
	training.Transition(runtime.READY)
	frames := market.ImpulseTape("BTC/USD", 6)
	b.ReportAllocs()

	for index := 0; b.Loop(); index++ {
		frame := frames[index%len(frames)]
		frame.SeqIdx = int64(index + 1)

		for _, observation := range frame.Peers {
			observation.SeqIdx = frame.SeqIdx
		}

		if output := training.Step(frame); output.Err != nil {
			b.Fatal(output.Err)
		}
	}
}
