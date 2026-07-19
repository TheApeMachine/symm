package types

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
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
	Convey("Given a trail that has earned a locked floor", t, func() {
		trail := NewTrail()
		trail.Bind(0.02)
		trail.Advance(0.20)

		So(trail.LockedFloor, ShouldBeGreaterThan, 0)

		Convey("When Scale widens then Advance holds the peak", func() {
			wide := trail.StopReturn
			trail.Scale(StopEvidence{
				Present:     true,
				Uncertainty: 0.10,
				Spread:      0.10,
			}, 1)
			trail.Advance(0.20)
			tight := trail.StopReturn

			So(tight, ShouldBeGreaterThanOrEqualTo, wide)

			Convey("And a later pullback must not loosen the raised stop", func() {
				trail.Scale(StopEvidence{
					Present:     true,
					Uncertainty: 0.02,
					Spread:      0.02,
				}, 1)
				trail.Advance(0.20)
				raised := trail.StopReturn
				trail.Advance(0.15)

				So(trail.StopReturn, ShouldBeGreaterThanOrEqualTo, raised)
			})
		})
	})
}

/*
TestTrailScaleRejectsNonFinite keeps NaN weight from disabling Breached.
*/
func TestTrailScaleRejectsNonFinite(t *testing.T) {
	Convey("Given a bound trail", t, func() {
		trail := NewTrail()
		trail.Bind(0.02)
		prior := trail.TrailDistance

		Convey("When Scale receives a non-finite weight", func() {
			trail.Scale(StopEvidence{Present: true, Uncertainty: 0.01}, math.NaN())

			Convey("Then TrailDistance is unchanged and finite", func() {
				So(trail.TrailDistance, ShouldEqual, prior)
				So(math.IsNaN(trail.TrailDistance), ShouldBeFalse)
			})
		})
	})
}

/*
TestTrailScaleRespectsFloorDistance keeps high skill from undercutting the
fill-time fee/spread survival band.
*/
func TestTrailScaleRespectsFloorDistance(t *testing.T) {
	Convey("Given a trail bound to a survival band", t, func() {
		trail := NewTrail()
		trail.Bind(0.0052)

		Convey("When Scale applies high skill with tight evidence", func() {
			trail.Scale(StopEvidence{
				Present:     true,
				Uncertainty: 0.001,
				Spread:      0.0005,
			}, 1)

			Convey("Then TrailDistance stays at or above FloorDistance", func() {
				So(trail.TrailDistance, ShouldBeGreaterThanOrEqualTo, 0.0052)
			})
		})
	})
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
