package numeric

import "testing"

func TestMedian(t *testing.T) {
	median := Median([]float64{1, 3, 9})

	if median != 3 {
		t.Fatalf("expected 3, got %v", median)
	}
}

func TestPercentileSorted(t *testing.T) {
	sorted := CopySorted([]float64{1, 2, 3, 4})

	if value := PercentileSorted(sorted, 0.5); value != 2.5 {
		t.Fatalf("expected 2.5, got %v", value)
	}
}

func TestMedianAbsoluteDeviation(t *testing.T) {
	sorted := CopySorted([]float64{1, 2, 3, 100})
	mad := MedianAbsoluteDeviation(sorted, Median(sorted))

	if mad <= 0 {
		t.Fatalf("expected positive MAD, got %v", mad)
	}
}

func BenchmarkMedian(b *testing.B) {
	values := CopySorted([]float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})

	for b.Loop() {
		_ = Median(values)
	}
}
