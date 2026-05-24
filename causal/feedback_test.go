package causal

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
)

func TestApplyPredictionFeedback(t *testing.T) {
	store := NewTrackStore()

	convey.Convey("Given overconfident settled feedback", t, func() {
		store.ApplyPredictionFeedback(engine.PredictionFeedback{
			Symbol:          "ALT/EUR",
			PredictedReturn: 0.2,
			ActualReturn:    0.1,
		})

		convey.Convey("It should lower intervention calibration", func() {
			store.mu.Lock()
			defer store.mu.Unlock()

			track := store.bySymbol["ALT/EUR"]

			convey.So(track.calibrator.Scale(), convey.ShouldAlmostEqual, 0.5, 0.0001)
		})
	})
}

func TestOpportunityRunway(t *testing.T) {
	samples := []causalSample{
		{priceVelocity: 0.01},
		{priceVelocity: 0.02},
		{priceVelocity: 0.08},
	}

	convey.Convey("Given excess velocity versus history", t, func() {
		convey.Convey("It should shorten the runway", func() {
			runway := opportunityRunway(samples, time.Second)

			convey.So(runway, convey.ShouldBeLessThan, time.Second)
			convey.So(runway, convey.ShouldBeGreaterThan, 0)
		})
	})
}
