package leadlag

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSymbolStatePriceSamples(t *testing.T) {
	Convey("Given ticker observations", t, func() {
		state := newSymbolState()
		start := time.Now()

		// Samples are spaced at the ring's decimation interval: pushes inside
		// ringSampleSpacing update last but do not enter the ring — that is the
		// fix that lets 256 slots actually span the 60m lag window.
		for index := range 20 {
			state.observeTicker(100+float64(index), start.Add(time.Duration(index)*ringSampleSpacing))
		}

		Convey("It should retain enough samples for correlation", func() {
			So(len(state.priceSamples()), ShouldBeGreaterThanOrEqualTo, minLagSamples)
		})
	})
}

func TestSymbolStateCrossLagInsufficientData(t *testing.T) {
	Convey("Given sparse histories", t, func() {
		anchor := newSymbolState()
		follower := newSymbolState()
		now := time.Now()

		anchor.observeTicker(100, now)
		follower.observeTicker(200, now)

		_, _, ok := follower.crossLag(anchor)

		Convey("It should refuse to score lag without enough samples", func() {
			So(ok, ShouldBeFalse)
		})
	})
}

func TestSymbolStateContemporaneous(t *testing.T) {
	Convey("Given aligned price paths", t, func() {
		anchor := newSymbolState()
		follower := newSymbolState()
		start := time.Now()

		for index := range 20 {
			at := start.Add(time.Duration(index) * ringSampleSpacing)
			anchor.observeTicker(100+float64(index), at)
			follower.observeTicker(200+float64(index)*2, at)
		}

		correlation, ok := follower.contemporaneous(anchor)

		Convey("It should compute positive contemporaneous correlation", func() {
			So(ok, ShouldBeTrue)
			So(correlation, ShouldBeGreaterThan, 0.5)
		})
	})
}
