package trader

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
)

func TestReturnModelApply(t *testing.T) {
	model := NewReturnModel()

	for range 12 {
		model.Apply(engine.PredictionFeedback{
			Source:          "hawkes",
			Regime:          "momentum",
			PredictedReturn: 0.01,
			ActualReturn:    0.02,
		})
	}

	gross, ok := model.Predict("hawkes", "momentum", 0.5)

	Convey("Given enough settled samples", t, func() {
		Convey("It should predict a positive gross return", func() {
			So(ok, ShouldBeTrue)
			So(gross, ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkReturnModelPredict(b *testing.B) {
	model := seedReturnModel("hawkes", "momentum", 0.02)

	b.ReportAllocs()

	for b.Loop() {
		_, _ = model.Predict("hawkes", "momentum", 0.5)
	}
}
