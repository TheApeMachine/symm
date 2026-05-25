package numeric

import (
	"math"
	"testing"
)

func TestLogSpaceCoversRange(t *testing.T) {
	values := LogSpace(1, 10, 5)

	if len(values) != 5 {
		t.Fatalf("expected five values, got %d", len(values))
	}

	if values[0] >= values[len(values)-1] {
		t.Fatal("expected ascending log-spaced values")
	}

	if math.Abs(values[0]-1) > 1e-9 {
		t.Fatalf("expected lower bound near 1, got %v", values[0])
	}
}

func TestLinSpaceCoversRange(t *testing.T) {
	values := LinSpace(0, 1, 3)

	if len(values) != 3 {
		t.Fatalf("expected three values, got %d", len(values))
	}

	if values[0] != 0 || values[2] != 1 {
		t.Fatalf("expected endpoints 0 and 1, got %v", values)
	}
}
