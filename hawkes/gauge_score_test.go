package hawkes

import "testing"

func TestGaugeScoreDoesNotPersistHistory(t *testing.T) {
	trackStore := NewTrackStore()
	track := trackStore.track("PUMP/EUR")
	track.confidenceHistory = []float64{1, 1.2, 1.4, 1.6, 1.8, 2.0, 2.2, 2.4}

	historyLen := len(track.confidenceHistory)

	score := trackStore.GaugeScore("PUMP/EUR", 2.5)

	if score <= 0 || score > 1 {
		t.Fatalf("expected unit-scale gauge score, got %v", score)
	}

	track.Lock()
	defer track.Unlock()

	if len(track.confidenceHistory) != historyLen {
		t.Fatalf("expected gauge score without history append, got len %d want %d",
			len(track.confidenceHistory), historyLen)
	}
}

func TestRecordScorePersistsHistory(t *testing.T) {
	trackStore := NewTrackStore()
	track := trackStore.track("PUMP/EUR")
	track.confidenceHistory = []float64{1, 1.2, 1.4, 1.6, 1.8, 2.0, 2.2, 2.4}

	historyLen := len(track.confidenceHistory)

	score := trackStore.RecordScore("PUMP/EUR", 2.5)

	if score <= 0 || score > 1 {
		t.Fatalf("expected unit-scale score, got %v", score)
	}

	track.Lock()
	defer track.Unlock()

	if len(track.confidenceHistory) != historyLen+1 {
		t.Fatalf("expected history append, got len %d want %d",
			len(track.confidenceHistory), historyLen+1)
	}
}
