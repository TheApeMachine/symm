package hawkes

import "testing"

func TestBeginScanClearsLiveScore(t *testing.T) {
	store := NewTrackStore()
	track := store.track("PUMP/EUR")
	track.Lock()
	track.liveScore = 1
	track.Unlock()

	store.BeginScan()

	if track.liveScore != 0 {
		t.Fatalf("expected live score reset, got %v", track.liveScore)
	}
}

func TestPeakLiveConfidenceReflectsCurrentTickOnly(t *testing.T) {
	store := NewTrackStore()

	first := store.track("AAA/EUR")
	first.Lock()
	first.liveScore = 1
	first.Unlock()

	store.BeginScan()

	second := store.track("BBB/EUR")
	second.Lock()
	second.liveScore = 0.25
	second.Unlock()

	if peak := store.PeakLiveConfidence(); peak != 0.25 {
		t.Fatalf("expected current tick peak 0.25, got %v", peak)
	}
}
