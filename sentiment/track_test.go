package sentiment

import "testing"

func TestSentimentRaw(t *testing.T) {
	if sentimentRaw(1.2, 0.8) <= 0 {
		t.Fatalf("expected positive sentiment score")
	}
}
