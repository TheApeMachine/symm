package leadlag

import "testing"

func TestLeadLagScore(t *testing.T) {
	if leadLagScore(0.02, 0.005) <= 0 {
		t.Fatalf("expected positive lead-lag score")
	}
}
