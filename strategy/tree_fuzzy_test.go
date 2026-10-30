package strategy

import "testing"

func TestPathSimilarity(t *testing.T) {
	if pathSimilarity("A/B/C", "A/B/C/enter.bin") != 1 {
		t.Fatalf("identical spellings should score 1")
	}
	if pathSimilarity("A/B", "A/C/enter.bin") <= 0 {
		t.Fatalf("shared prefix should score positive")
	}
}
