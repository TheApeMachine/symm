package toxicity

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

func TestSignalStep(t *testing.T) {
	Convey("Given measurements produced at the book's transport boundary", t, func() {
		signal := NewSignal(t.Context())
		Reset(func() { So(signal.Close(), ShouldBeNil) })
		measurement := data.NewMeasurement[float64]("", "TEST/USD", "toxicity", time.Time{}, time.Time{})
		envelope := types.NewEnvelope(types.EnvelopeLevel3)
		envelope.Level3Data.Symbol = "TEST/USD"
		envelope.Toxicity = measurement
		So(signal.Step(envelope), ShouldEqual, envelope)
		So(envelope.Toxicity, ShouldEqual, measurement)
		So(signal.Error(), ShouldBeNil)
	})
}

func BenchmarkSignalStep(b *testing.B) {
	signal := NewSignal(b.Context())
	b.Cleanup(func() {
		if err := signal.Close(); err != nil {
			b.Error(err)
		}
	})
	envelope := types.NewEnvelope(types.EnvelopeLevel3)
	envelope.Level3Data.Symbol = "TEST/USD"
	b.ReportAllocs()
	for b.Loop() {
		signal.Step(envelope)
	}
}
