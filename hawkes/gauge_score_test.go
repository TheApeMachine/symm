package hawkes

import (
	"testing"

	"github.com/theapemachine/symm/engine"
)

func TestGaugeScoreDoesNotPersistHistory(t *testing.T) {
	sym := NewHawkesSymbol(engine.DefaultCalibrationParams())
	sym.confidenceHistory = []float64{1, 1.2, 1.4, 1.6, 1.8, 2.0, 2.2, 2.4}

	historyLen := len(sym.confidenceHistory)

	score := sym.gaugeScore(2.5, false)

	if score <= 0 || score > 1 {
		t.Fatalf("expected unit-scale gauge score, got %v", score)
	}

	if len(sym.confidenceHistory) != historyLen {
		t.Fatalf("expected gauge score without history append, got len %d want %d",
			len(sym.confidenceHistory), historyLen)
	}
}

func TestRecordScorePersistsHistory(t *testing.T) {
	sym := NewHawkesSymbol(engine.DefaultCalibrationParams())
	sym.confidenceHistory = []float64{1, 1.2, 1.4, 1.6, 1.8, 2.0, 2.2, 2.4}

	historyLen := len(sym.confidenceHistory)

	score := sym.gaugeScore(2.5, true)

	if score <= 0 || score > 1 {
		t.Fatalf("expected unit-scale score, got %v", score)
	}

	if len(sym.confidenceHistory) != historyLen+1 {
		t.Fatalf("expected history append, got len %d want %d",
			len(sym.confidenceHistory), historyLen+1)
	}
}
