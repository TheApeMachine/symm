package pumpdump

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
)

func calibratedConfidenceHistory() []float64 {
	return []float64{0.2, 0.4, 0.6, 0.8, 1.0, 1.2, 1.4, 1.6, 1.8, 2.0, 2.2, 2.4}
}

func TestFinalizeReading(t *testing.T) {
	trackStore, _ := testTrackStore(t)

	convey.Convey("Given a raw precursor score", t, func() {
		track := trackStore.ensure("PUMP/EUR")
		track.confidenceHistory = calibratedConfidenceHistory()

		convey.Convey("It should normalize and record confidence history", func() {
			confidence, reason := FinalizeReading(
				trackStore, "PUMP/EUR", 0.5, "actual_pump",
			)

			convey.So(reason, convey.ShouldEqual, "actual_pump")
			convey.So(confidence, convey.ShouldBeGreaterThan, 0)
			convey.So(confidence, convey.ShouldBeLessThanOrEqualTo, 1)
		})
	})
}

func TestFinalizeReadingSetsLiveScore(t *testing.T) {
	trackStore, _ := testTrackStore(t)
	track := trackStore.ensure("PUMP/EUR")
	track.confidenceHistory = calibratedConfidenceHistory()

	confidence, _ := FinalizeReading(trackStore, "PUMP/EUR", 0.5, "actual_pump")

	if confidence <= 0 {
		t.Fatalf("expected normalized confidence, got %v", confidence)
	}

	if track.liveScore <= 0 {
		t.Fatalf("expected live gauge score, got %v", track.liveScore)
	}
}

func TestResetLiveScores(t *testing.T) {
	trackStore, _ := testTrackStore(t)
	track := trackStore.ensure("PUMP/EUR")
	track.liveScore = 0.75

	trackStore.ResetLiveScores()

	if track.liveScore != 0 {
		t.Fatalf("expected live score reset, got %v", track.liveScore)
	}
}

func TestApplyPredictionFeedback(t *testing.T) {
	trackStore, _ := testTrackStore(t)

	convey.Convey("Given overconfident settled feedback", t, func() {
		trackStore.ApplyPredictionFeedback(engine.PredictionFeedback{
			Symbol:          "PUMP/EUR",
			PredictedReturn: 0.2,
			ActualReturn:    0.1,
		})

		convey.Convey("It should lower precursor calibration", func() {
			convey.So(trackStore.CalibrationScale("PUMP/EUR"), convey.ShouldAlmostEqual, 0.5, 0.0001)
		})
	})
}
