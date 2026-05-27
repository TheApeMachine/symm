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

func TestFeedbackIncludesSource(t *testing.T) {
	feedback := PredictionFeedback{
		Source:  PerspectiveSource(PerspectiveMicrostructure),
		Sources: []string{"pumpdump", "hawkes"},
		Symbol:  "BTC/EUR",
	}

	if !FeedbackIncludesSource(feedback, "pumpdump") {
		t.Fatal("expected perspective feedback to include pumpdump")
	}

	if FeedbackIncludesSource(feedback, "depthflow") {
		t.Fatal("expected unrelated source to be excluded")
	}
}
