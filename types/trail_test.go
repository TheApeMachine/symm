package types

import (
	"math"
	"testing"
)

/*
TestTrailAdvanceLocksFloorAfterEarnedCushion proves Advance leaves −Inf until
peak clears trail distance, then locks a positive floor.
*/
func TestTrailAdvanceLocksFloorAfterEarnedCushion(t *testing.T) {
	t.Parallel()

	trail := NewTrail()
	trail.Bind(0.02)
	trail.Advance(0.01)

	if !math.IsInf(trail.LockedFloor, -1) {
		t.Fatalf("unearned peak must keep -Inf floor, got %v", trail.LockedFloor)
	}

	trail.Advance(0.05)

	if trail.LockedFloor <= 0 {
		t.Fatalf("earned peak must lock floor, got %v", trail.LockedFloor)
	}
}

/*
TestTrailBreachedReportsCrossThroughLiveStop checks the breach predicate.
*/
func TestTrailBreachedReportsCrossThroughLiveStop(t *testing.T) {
	t.Parallel()

	trail := NewTrail()
	trail.Bind(0.02)

	if trail.Breached(-0.01) {
		t.Fatal("mark above stop must not breach")
	}

	if !trail.Breached(-0.03) {
		t.Fatal("mark through stop must breach")
	}
}

/*
BenchmarkTrailAdvance measures one Advance for the hot mark path.
*/
func BenchmarkTrailAdvance(b *testing.B) {
	trail := NewTrail()
	trail.Bind(0.02)

	b.ReportAllocs()
	b.ResetTimer()

	for index := 0; b.Loop(); index++ {
		trail.Advance(float64(index%50) * 0.001)
	}
}
