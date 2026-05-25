package stats

import "testing"

func TestMedianAbsolute(t *testing.T) {
	median := MedianAbsolute([]float64{-3, -1, 2, 4})

	if median != 2.5 {
		t.Fatalf("expected median absolute 2.5, got %v", median)
	}
}
