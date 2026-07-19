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
TestTrailAdvanceNeverLoosensStopAfterFloor proves that once a cushion exists,
Scale-tightening then a pullback cannot lower StopReturn below the prior live
stop — the trailing ratchet must be monotone.
*/
func TestTrailAdvanceNeverLoosensStopAfterFloor(t *testing.T) {
	t.Parallel()

	trail := NewTrail()
	trail.Bind(0.02)
	trail.Advance(0.20)

	if trail.LockedFloor <= 0 {
		t.Fatalf("want earned floor, got %v", trail.LockedFloor)
	}

	wide := trail.StopReturn
	trail.Scale(StopEvidence{
		Present:     true,
		Uncertainty: 0.10,
		Spread:      0.10,
	}, 1)
	trail.Advance(0.20)
	tight := trail.StopReturn

	if tight < wide {
		t.Fatalf("wider scale must not drop stop below prior: wide=%v tight=%v", wide, tight)
	}

	trail.Scale(StopEvidence{
		Present:     true,
		Uncertainty: 0.02,
		Spread:      0.02,
	}, 1)
	trail.Advance(0.20)
	raised := trail.StopReturn
	trail.Advance(0.15)

	if trail.StopReturn < raised {
		t.Fatalf(
			"pullback must not loosen stop: raised=%v after=%v",
			raised, trail.StopReturn,
		)
	}
}

/*
TestTrailScaleRejectsNonFinite keeps NaN weight from disabling Breached.
*/
func TestTrailScaleRejectsNonFinite(t *testing.T) {
	t.Parallel()

	trail := NewTrail()
	trail.Bind(0.02)
	prior := trail.TrailDistance
	trail.Scale(StopEvidence{Present: true, Uncertainty: 0.01}, math.NaN())

	if trail.TrailDistance != prior || math.IsNaN(trail.TrailDistance) {
		t.Fatalf("non-finite weight must leave TrailDistance=%v, got %v", prior, trail.TrailDistance)
	}
}

/*
TestTrailScaleRespectsFloorDistance keeps high skill from undercutting the
fill-time fee/spread survival band.
*/
func TestTrailScaleRespectsFloorDistance(t *testing.T) {
	t.Parallel()

	trail := NewTrail()
	trail.Bind(0.0052)
	trail.Scale(StopEvidence{
		Present:     true,
		Uncertainty: 0.001,
		Spread:      0.0005,
	}, 1)

	if trail.TrailDistance < 0.0052 {
		t.Fatalf(
			"Scale must not undercut FloorDistance: got %v floor %v",
			trail.TrailDistance, trail.FloorDistance,
		)
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
