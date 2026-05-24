package hawkes

import (
	"testing"
	"time"
)

func TestExcitationRunway(t *testing.T) {
	runway := excitationRunway(BivariateFit{Beta: 4})

	if runway != 250*time.Millisecond {
		t.Fatalf("expected 250ms e-folding runway, got %v", runway)
	}
}

func TestExcitationRunwayZeroBeta(t *testing.T) {
	if excitationRunway(BivariateFit{}) != 0 {
		t.Fatal("expected zero runway when beta is unset")
	}
}
