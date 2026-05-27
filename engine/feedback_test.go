package engine

import "testing"

func TestValidPredictionFeedback(t *testing.T) {
	if ValidPredictionFeedback(PredictionFeedback{
		Source:          "pumpdump",
		Symbol:          "PUMP/EUR",
		PredictedReturn: 0.01,
		Unanchored:      true,
	}) {
		t.Fatal("expected unanchored feedback to be rejected")
	}

	if !ValidPredictionFeedback(PredictionFeedback{
		Source:          "pumpdump",
		Symbol:          "PUMP/EUR",
		PredictedReturn: 0.01,
		Unanchored:      false,
	}) {
		t.Fatal("expected anchored positive forecast to pass")
	}

	if !ValidPredictionFeedback(PredictionFeedback{
		Source:          "pumpdump",
		Symbol:          "PUMP/EUR",
		PredictedReturn: 0,
		Unanchored:      false,
	}) {
		t.Fatal("expected zero predicted return to pass when anchored")
	}
}
