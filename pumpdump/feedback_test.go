package pumpdump

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
)

func TestFinalizeMeasurement(t *testing.T) {
	store := NewTrackStore()
	now := time.Unix(1_700_000_000, 0)

	convey.Convey("Given a raw precursor score", t, func() {
		track := store.ensure("PUMP/EUR")
		track.bucketStart = now
		track.confidenceHistory = []float64{0.2, 0.4, 0.6}

		track.priceMoves = []float64{0.02, 0.015, 0.018}

		convey.Convey("It should normalize and return bucket runway", func() {
			confidence, expectedReturn, runway, reason := store.FinalizeMeasurement("PUMP/EUR", 0.5, now.Add(time.Minute), "precursor")

			convey.So(reason, convey.ShouldEqual, "precursor")
			convey.So(runway, convey.ShouldEqual, 4*time.Minute)
			convey.So(confidence, convey.ShouldBeGreaterThan, 0)
			convey.So(confidence, convey.ShouldBeLessThanOrEqualTo, 1)
			convey.So(expectedReturn, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestFinalizeMeasurementSetsLiveScoreWithoutRunway(t *testing.T) {
	store := NewTrackStore()
	track := store.ensure("PUMP/EUR")
	track.confidenceHistory = []float64{0.2, 0.4, 0.6}

	_, _, runway, _ := store.FinalizeMeasurement(
		"PUMP/EUR",
		0.5,
		time.Unix(1_700_000_000, 0),
		"precursor",
	)

	if runway != 0 {
		t.Fatalf("expected zero runway, got %v", runway)
	}

	if track.liveScore <= 0 {
		t.Fatalf("expected live gauge score without runway, got %v", track.liveScore)
	}
}

func TestBeginScanClearsLiveScore(t *testing.T) {
	store := NewTrackStore()
	track := store.ensure("PUMP/EUR")
	track.liveScore = 0.75

	store.BeginScan()

	if track.liveScore != 0 {
		t.Fatalf("expected live score reset, got %v", track.liveScore)
	}
}

func TestApplyPredictionFeedback(t *testing.T) {
	store := NewTrackStore()

	convey.Convey("Given overconfident settled feedback", t, func() {
		store.ApplyPredictionFeedback(engine.PredictionFeedback{
			Symbol:          "PUMP/EUR",
			PredictedReturn: 0.2,
			ActualReturn:    0.1,
		})

		convey.Convey("It should lower precursor calibration", func() {
			convey.So(store.CalibrationScale("PUMP/EUR"), convey.ShouldAlmostEqual, 0.5, 0.0001)
		})
	})
}
