package manifold

import (
	"math"
	"testing"
)

func TestReturnFrequencyReturnsZeroForZeroDelta(t *testing.T) {
	frequency := returnFrequency(nil, 0)

	if math.IsNaN(frequency) || math.IsInf(frequency, 0) {
		t.Fatalf("frequency = %g, want finite", frequency)
	}

	if frequency != 0 {
		t.Fatalf("frequency = %g, want 0", frequency)
	}
}
