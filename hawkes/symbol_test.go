package hawkes

import (
	"testing"

	"github.com/theapemachine/symm/engine"
)

func TestHawkesSymbolApplyFeedback(t *testing.T) {
	symbol := NewHawkesSymbol(engine.DefaultCalibrationParams())
	symbol.fit = sampleFit()
	symbol.hasFit = true

	symbol.ApplyFeedback(engine.PredictionFeedback{
		Source:          hawkesSource,
		Symbol:          "PUMP/EUR",
		PredictedReturn: 0.1,
		ActualReturn:    0.05,
	})

	if symbol.calibrator.Scale() >= 1 {
		t.Fatalf("expected calibration scale below 1 after overconfident miss, got %v", symbol.calibrator.Scale())
	}
}
