package hawkes

import "testing"

func TestBeginScanClearsLiveScore(t *testing.T) {
	store := NewTrackStore()
	track := store.ensure("PUMP/EUR")
	track.liveScore = 1

	store.BeginScan()

	if track.liveScore != 0 {
		t.Fatalf("expected live score reset, got %v", track.liveScore)
	}
}

func TestPeakLiveConfidenceReflectsCurrentTickOnly(t *testing.T) {
	store := NewTrackStore()

	first := store.ensure("AAA/EUR")
	first.liveScore = 1

	store.BeginScan()

	second := store.ensure("BBB/EUR")
	second.liveScore = 0.25

	if peak := store.PeakLiveConfidence(); peak != 0.25 {
		t.Fatalf("expected current tick peak 0.25, got %v", peak)
	}
}
