package engine

import "testing"

func TestScoreRecorderRecord(t *testing.T) {
	recorder := NewScoreRecorder(DefaultCalibrationParams(), 8)

	for index := 0; index < 8; index++ {
		recorder.ConfidenceHist = append(recorder.ConfidenceHist, 1+float64(index)*0.2)
	}

	score := recorder.Record(2.5, nil)

	if score <= 0 || score > 1 {
		t.Fatalf("expected unit-scale score, got %v", score)
	}

	if len(recorder.ConfidenceHist) != recorder.HistoryCap {
		t.Fatalf("expected capped history len %d, got %d", recorder.HistoryCap, len(recorder.ConfidenceHist))
	}
}
