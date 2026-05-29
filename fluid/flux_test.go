package fluid

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFluxAccumulatorClosesBarOnVolume(t *testing.T) {
	Convey("Given a volume-clocked accumulator with a target of 10 base units", t, func() {
		flux := newFluxAccumulator(time.Hour)
		flux.setTarget(10)
		now := time.Unix(1_700_000_000, 0)

		flux.addBook(now, 4)
		flux.addTrade(now, 6)
		flux.addTrade(now.Add(time.Millisecond), 3)

		Convey("Before the target is reached the open bar is read", func() {
			So(flux.tradeFlux(), ShouldEqual, 9)
			So(flux.bookFlux(), ShouldEqual, 4)
		})

		Convey("The fill that crosses the target completes the bar", func() {
			flux.addTrade(now.Add(2*time.Millisecond), 5)

			// Completed bar held 9+5 = 14 of fills and 4 of churn; a fresh bar is now open.
			So(flux.tradeFlux(), ShouldEqual, 14)
			So(flux.bookFlux(), ShouldEqual, 4)

			flux.addTrade(now.Add(3*time.Millisecond), 1)
			flux.addBook(now.Add(3*time.Millisecond), 2)

			// Still reading the last completed bar, not the new partial one.
			So(flux.tradeFlux(), ShouldEqual, 14)
			So(flux.bookFlux(), ShouldEqual, 4)
		})
	})
}

func TestFluxAccumulatorFallsBackToWallClock(t *testing.T) {
	Convey("Given an accumulator with no volume target", t, func() {
		flux := newFluxAccumulator(10 * time.Second)
		now := time.Unix(1_700_000_000, 0)

		flux.addBook(now, 5)
		flux.addTrade(now, 3)

		Convey("A bar still closes once the max age elapses", func() {
			So(flux.tradeFlux(), ShouldEqual, 3)

			flux.addTrade(now.Add(11*time.Second), 7)

			// The aged bar (3 fills, 5 churn) is force-closed before the late fill is folded, so
			// that fill opens the next bar and reading still returns the completed bar.
			So(flux.tradeFlux(), ShouldEqual, 3)
			So(flux.bookFlux(), ShouldEqual, 5)
		})
	})
}

func TestFluxAccumulatorVolumeClockAcceleratesUnderActivity(t *testing.T) {
	Convey("Given a tight volume target", t, func() {
		flux := newFluxAccumulator(time.Hour)
		flux.setTarget(2)
		now := time.Unix(1_700_000_000, 0)

		Convey("A burst of fills closes several bars within the same instant", func() {
			flux.addTrade(now, 2) // closes bar 1
			flux.addTrade(now, 2) // closes bar 2
			flux.addTrade(now, 2) // closes bar 3

			So(flux.tradeFlux(), ShouldEqual, 2)
		})
	})
}
