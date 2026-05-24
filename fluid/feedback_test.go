package fluid

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
)

func TestApplyPredictionFeedback(t *testing.T) {
	store := NewTrackStore(engine.DefaultCalibrationParams())

	convey.Convey("Given overconfident settled feedback", t, func() {
		store.ApplyPredictionFeedback(engine.PredictionFeedback{
			Symbol:          "PUMP/EUR",
			PredictedReturn: 0.2,
			ActualReturn:    0.1,
		})

		convey.Convey("It should lower source/shock calibration", func() {
			track := store.track("PUMP/EUR")
			track.Lock()
			defer track.Unlock()

			convey.So(track.calibrator.Scale(), convey.ShouldAlmostEqual, 0.5, 0.0001)
		})
	})
}
