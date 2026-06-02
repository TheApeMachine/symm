package causal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRegimeTrackerHysteresis(t *testing.T) {
	Convey("Given a regime tracker with hysteresis of three", t, func() {
		tracker := regimeTracker{}
		hysteresis := 3

		Convey("When panic trips arrive one at a time", func() {
			first := tracker.apply(true, hysteresis)
			second := tracker.apply(true, hysteresis)
			third := tracker.apply(true, hysteresis)

			Convey("It should not invert until the threshold is met", func() {
				So(first, ShouldBeFalse)
				So(second, ShouldBeFalse)
				So(third, ShouldBeTrue)
			})
		})

		Convey("When panic trips oscillate before the threshold", func() {
			_ = tracker.apply(true, hysteresis)
			_ = tracker.apply(true, hysteresis)
			stillNormal := tracker.apply(false, hysteresis)
			againPanic := tracker.apply(true, hysteresis)

			Convey("It should reset the pending panic counter", func() {
				So(stillNormal, ShouldBeFalse)
				So(againPanic, ShouldBeFalse)
				So(tracker.invertedNow(), ShouldBeFalse)
			})
		})
	})
}

func TestDeriveRegimeHysteresisSamples(t *testing.T) {
	Convey("Given causal history length", t, func() {
		Convey("It should derive at least two consecutive samples", func() {
			So(deriveRegimeHysteresisSamples(minCausalHistory+12), ShouldBeGreaterThan, 1)
		})
	})
}

func BenchmarkRegimeTrackerApply(b *testing.B) {
	tracker := regimeTracker{}
	hysteresis := deriveRegimeHysteresisSamples(minCausalHistory + 8)

	b.ReportAllocs()

	for b.Loop() {
		_ = tracker.apply(true, hysteresis)
		_ = tracker.apply(false, hysteresis)
	}
}
