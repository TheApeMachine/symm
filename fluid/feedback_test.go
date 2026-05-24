package fluid

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
)

func TestApplyPredictionFeedback(t *testing.T) {
	store := NewTrackStore()

	convey.Convey("Given overconfident settled feedback", t, func() {
		store.ApplyPredictionFeedback(engine.PredictionFeedback{
			Symbol:          "PUMP/EUR",
			PredictedReturn: 0.2,
			ActualReturn:    0.1,
		})

		convey.Convey("It should lower source/shock calibration", func() {
			store.mu.Lock()
			defer store.mu.Unlock()

			track := store.bySymbol["PUMP/EUR"]

			convey.So(track.calibrator.Scale(), convey.ShouldAlmostEqual, 0.5, 0.0001)
		})
	})
}
