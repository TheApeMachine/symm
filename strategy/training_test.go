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
		tape := make(chan [][]*data.Measurement[float64])
		close(tape)
		training := NewTraining(context.Background(), tape)
		envelope := &types.Envelope{}
		So(training.Step(envelope), ShouldEqual, envelope)
		So(envelope.Learning, ShouldEqual, training)
		So(training.Error(), ShouldBeNil)
	})

	Convey("A tape frame flows through the envelope-typed composition", t, func() {
		tape := make(chan [][]*data.Measurement[float64], 1)
		measurement := data.NewMeasurement[float64](
			"1", "BTC/USD", "cvd", time.Now().UTC(), time.Time{},
		)
		measurement.PutMetric(data.Metric[float64]{Label: "signed", Raw: 1.5})
		tape <- [][]*data.Measurement[float64]{{measurement}}
		close(tape)
		training := NewTraining(context.Background(), tape)
		envelope := &types.Envelope{}
		So(training.Step(envelope), ShouldEqual, envelope)
		So(len(envelope.Impulses), ShouldEqual, 1)
		So(envelope.Impulses[0].Label, ShouldEqual, "BTC/USD")
		So(training.Error(), ShouldBeNil)
	})
}
