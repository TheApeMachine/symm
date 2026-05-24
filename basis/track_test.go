package basis

import "testing"

func TestBasisScore(t *testing.T) {
	if basisScore(2, 1) <= 0 {
		t.Fatalf("expected positive basis score")
	}
}
