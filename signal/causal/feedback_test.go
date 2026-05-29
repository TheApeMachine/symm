package causal

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

func TestApplyPredictionFeedback(t *testing.T) {
	state := NewCausalSymbol(asset.Pair{Wsname: "ALT/EUR"}, engine.DefaultCalibrationParams())

	convey.Convey("Given overconfident settled feedback", t, func() {
		state.ApplyFeedback(engine.PredictionFeedback{
			Source:          causalSource,
			Symbol:          "ALT/EUR",
			PredictedReturn: 0.2,
			ActualReturn:    0.1,
		})

		convey.Convey("It should lower intervention calibration", func() {
			convey.So(state.calibrator.Scale(), convey.ShouldAlmostEqual, 0.5, 0.0001)
		})
	})
}
