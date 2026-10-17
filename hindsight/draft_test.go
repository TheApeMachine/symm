package hindsight

import "testing"

func TestIncludedRetrace(t *testing.T) {
	t.Parallel()

	// 1.00 up to 1.20, a dip to 1.05, then on to 1.40.
	// The swing is 0.40 and the dip is 0.15, so the fraction is 0.375.
	got := includedRetrace([]float64{1, 1.2, 1.05, 1.4})

	if got < 0.37 || got > 0.38 {
		t.Fatalf("includedRetrace() = %v, want about 0.375", got)
	}

	if includedRetrace([]float64{1}) != 0 {
		t.Fatal("a single print has no retrace")
	}
}
