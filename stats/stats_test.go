package stats

import "testing"

func TestMedian(t *testing.T) {
	if got := Median([]float64{3, 1, 2}); got != 2 {
		t.Fatalf("expected median 2, got %v", got)
	}
}

func TestPercentileSorted(t *testing.T) {
	sorted := []float64{1, 2, 3, 4}

	if got := PercentileSorted(sorted, 0.5); got != 2.5 {
		t.Fatalf("expected 2.5, got %v", got)
	}
}

func TestQuartiles(t *testing.T) {
	lower, upper := Quartiles([]float64{1, 2, 3, 4, 5})

	if lower != 2 || upper != 4 {
		t.Fatalf("expected quartiles 2 and 4, got %v %v", lower, upper)
	}
}

func TestMedianAbsoluteDeviation(t *testing.T) {
	sorted := CopySorted([]float64{1, 2, 3, 4, 5})
	mad := MedianAbsoluteDeviation(sorted, Median([]float64{1, 2, 3, 4, 5}))

	if mad <= 0 {
		t.Fatalf("expected positive MAD, got %v", mad)
	}
}
