package types

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestTrailAdvanceLocksFloorAfterEarnedCushion proves Advance leaves −Inf until
peak clears the survival band, then locks a positive floor.
*/
func TestTrailAdvanceLocksFloorAfterEarnedCushion(t *testing.T) {
	Convey("Given a bound trail and a peak still inside the survival band", t, func() {
		trail := NewTrail()
		trail.Bind(0.02)
		trail.Advance(0.01)

		Convey("Then LockedFloor stays unlocked but the stop has already risen", func() {
			So(math.IsInf(trail.LockedFloor, -1), ShouldBeTrue)
			So(trail.StopReturn, ShouldBeGreaterThan, -0.02)
			So(trail.StopReturn, ShouldAlmostEqual, -0.01, 1e-12)
		})

		Convey("When peak clears the survival band", func() {
			trail.Advance(0.05)

			Convey("Then LockedFloor locks above entry", func() {
				So(trail.LockedFloor, ShouldBeGreaterThan, 0)
				So(trail.StopReturn, ShouldBeGreaterThan, 0)
			})
		})
	})
}

/*
TestTrailAdvanceRatchetsBelowEntryOnPartialPeak proves a BILL-like runner on a
wide survival band raises StopReturn toward entry before the floor locks above
zero — the stop must not stay glued at −TrailDistance until a full-band peak.
*/
func TestTrailAdvanceRatchetsBelowEntryOnPartialPeak(t *testing.T) {
	Convey("Given a wide fill-time survival band", t, func() {
		trail := NewTrail()
		trail.Bind(0.057)

		Convey("When mark peaks at +2.2% (below the 5.7% band)", func() {
			trail.Advance(0.022)

			Convey("Then the stop ratchets up under the peak without locking above entry", func() {
				So(math.IsInf(trail.LockedFloor, -1), ShouldBeTrue)
				So(trail.StopReturn, ShouldAlmostEqual, 0.022-0.057, 1e-12)
				So(trail.StopReturn, ShouldBeGreaterThan, -0.057)
				So(trail.StopReturn, ShouldBeLessThan, 0)
			})
		})
	})
}

/*
TestTrailBreachedReportsCrossThroughLiveStop checks the breach predicate.
*/
func TestTrailBreachedReportsCrossThroughLiveStop(t *testing.T) {
	Convey("Given a bound trail", t, func() {
		trail := NewTrail()
		trail.Bind(0.02)

		Convey("Then marks above the stop do not breach", func() {
			So(trail.Breached(-0.01), ShouldBeFalse)
		})

		Convey("And marks through the stop breach", func() {
			So(trail.Breached(-0.03), ShouldBeTrue)
		})
	})
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
