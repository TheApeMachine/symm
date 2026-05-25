package engine

import "testing"

func TestMinCompletedLength(t *testing.T) {
	length := MinCompletedLength(map[string][]OHLCCandle{
		"AAA/EUR": {{}, {}, {}},
		"BBB/EUR": {{}, {}},
	})

	if length != 1 {
		t.Fatalf("expected shortest completed length 1, got %d", length)
	}
}
