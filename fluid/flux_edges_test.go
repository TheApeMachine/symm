package fluid

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFluxAccumulatorIgnoresInvalidInput(t *testing.T) {
	Convey("Given a fresh accumulator", t, func() {
		flux := newFluxAccumulator(time.Minute)
		now := time.Unix(1_700_000_000, 0)

		flux.setTarget(-5)
		flux.addBook(now, 0)
		flux.addTrade(now, 0)

		Convey("It should clamp negative targets and ignore zero churn", func() {
			So(flux.target, ShouldEqual, 0)
			So(flux.bookFlux(), ShouldEqual, 0)
			So(flux.tradeFlux(), ShouldEqual, 0)
		})
	})
}

func TestFluxAccumulatorOpenBarBeforeClose(t *testing.T) {
	Convey("Given an open bar with no completed bucket", t, func() {
		flux := newFluxAccumulator(time.Hour)
		now := time.Unix(1_700_000_000, 0)

		flux.addBook(now, 3)
		flux.addTrade(now, 2)

		Convey("It should read the in-progress bar", func() {
			So(flux.haveClosed, ShouldBeFalse)
			So(flux.bookFlux(), ShouldEqual, 3)
			So(flux.tradeFlux(), ShouldEqual, 2)
		})
	})
}

func BenchmarkFluxSetTarget(b *testing.B) {
	flux := newFluxAccumulator(time.Hour)

	for b.Loop() {
		flux.setTarget(1.5)
	}
}
