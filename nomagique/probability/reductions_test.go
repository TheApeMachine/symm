package probability

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique/types"
)

func TestGeomean(t *testing.T) {
	// The geometric mean of 1, 2, 4 is the cube root of 8, which is 2.
	if got := Geomean([]types.Scalar{1, 2, 4}); math.Abs(float64(got)-2) > 1e-12 {
		t.Fatalf("Geomean = %v, want 2", got)
	}

	// A single zero member zeroes the whole product — the property that makes
	// the geometric mean the right aggregate for multiplicative evidence.
	if got := Geomean([]types.Scalar{1, 2, 0, 4}); got != 0 {
		t.Fatalf("Geomean with a zero member = %v, want 0", got)
	}

	if got := Geomean([]types.Scalar{}); got != 0 {
		t.Fatalf("Geomean of nothing = %v, want 0", got)
	}

	if got := Geomean([]types.Scalar{1, -2}); got != 0 {
		t.Fatalf("Geomean with a negative member = %v, want 0", got)
	}
}

/*
TestGeomeanIsBelowArithmeticMean proves the aggregate genuinely penalizes
spread rather than behaving like a plain mean.
*/
func TestGeomeanIsBelowArithmeticMean(t *testing.T) {
	values := []types.Scalar{0.1, 0.9}

	arithmetic := types.Scalar(0.5)

	if Geomean(values) >= arithmetic {
		t.Fatalf("Geomean %v must fall below the arithmetic mean %v for spread evidence",
			Geomean(values), arithmetic)
	}
}

/*
TestGeomeanResistsUnderflow proves the log-space accumulation: a long
collection of small values must not collapse to zero before the root.
*/
func TestGeomeanResistsUnderflow(t *testing.T) {
	values := make([]types.Scalar, 500)

	for index := range values {
		values[index] = 1e-3
	}

	got := Geomean(values)

	if math.Abs(float64(got)-1e-3) > 1e-12 {
		t.Fatalf("Geomean = %v, want 1e-3 — the product underflowed", got)
	}
}

func TestShannonAmbiguity(t *testing.T) {
	// Uniform mass is maximally ambiguous.
	if got := ShannonAmbiguity([]types.Scalar{1, 1, 1, 1}); math.Abs(float64(got)-1) > 1e-12 {
		t.Fatalf("uniform ambiguity = %v, want 1", got)
	}

	// All mass on one member is unambiguous.
	if got := ShannonAmbiguity([]types.Scalar{5, 0, 0}); got != 0 {
		t.Fatalf("concentrated ambiguity = %v, want 0", got)
	}

	// Unnormalized weights must give the same reading as normalized ones.
	normalized := ShannonAmbiguity([]types.Scalar{0.25, 0.25, 0.5})
	unnormalized := ShannonAmbiguity([]types.Scalar{1, 1, 2})

	if math.Abs(float64(normalized-unnormalized)) > 1e-12 {
		t.Fatalf("ambiguity %v differs from its unnormalized form %v",
			normalized, unnormalized)
	}

	// A single member has nothing to be ambiguous between.
	if got := ShannonAmbiguity([]types.Scalar{7}); got != 0 {
		t.Fatalf("single-member ambiguity = %v, want 0", got)
	}

	if got := ShannonAmbiguity([]types.Scalar{0, 0}); got != 0 {
		t.Fatalf("massless ambiguity = %v, want 0", got)
	}
}

func TestEvidenceShare(t *testing.T) {
	values := []types.Scalar{1, 3}

	if got := EvidenceShare(values, 0); math.Abs(float64(got)-0.25) > 1e-12 {
		t.Fatalf("share = %v, want 0.25", got)
	}

	if got := EvidenceShare(values, 1); math.Abs(float64(got)-0.75) > 1e-12 {
		t.Fatalf("share = %v, want 0.75", got)
	}

	if got := EvidenceShare(values, 5); got != 0 {
		t.Fatalf("out-of-range share = %v, want 0", got)
	}

	if got := EvidenceShare([]types.Scalar{0, 0}, 0); got != 0 {
		t.Fatalf("massless share = %v, want 0", got)
	}
}

func TestArgmax(t *testing.T) {
	index, value, ok := Argmax([]types.Scalar{1, 9, 3})

	if !ok || index != 1 || value != 9 {
		t.Fatalf("Argmax = (%d, %v, %v), want (1, 9, true)", index, value, ok)
	}

	// Ties resolve to the first member so the reading is deterministic.
	index, _, _ = Argmax([]types.Scalar{4, 4})

	if index != 0 {
		t.Fatalf("tie resolved to %d, want 0", index)
	}

	if _, _, ok := Argmax(nil); ok {
		t.Fatal("an empty collection must have no argmax")
	}
}
