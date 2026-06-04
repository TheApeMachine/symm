package exhaust

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/ring"
)

func TestHistoryStoreObserve(t *testing.T) {
	Convey("Given a history store", t, func() {
		store := newHistoryStore()
		store.observe("BTC/EUR", 10, 12, 5, 8, 0.6, 0.2, 100)

		Convey("It should accumulate per-symbol samples", func() {
			store.mu.RLock()
			history := store.bySymbol["BTC/EUR"]
			store.mu.RUnlock()

			So(history, ShouldNotBeNil)
			So(history.lastPrice, ShouldEqual, 100)
			So(history.bidDepths, ShouldHaveSameTypeAs, ring.FloatRing{})
		})
	})
}

func TestPressureSmoothing(t *testing.T) {
	Convey("Given raw ±1 trade signs fed as buy pressure", t, func() {
		store := newHistoryStore()

		// A build-up of buying that then fades into selling — the trajectory
		// pressureFade is meant to read.
		for _, sign := range []float64{1, 1, 1, 1, 1, -1, -1, -1} {
			store.observe("BTC/EUR", 0, 0, 0, 0, sign, 0, 10)
		}

		store.mu.RLock()
		pressures := store.bySymbol["BTC/EUR"].pressures
		store.mu.RUnlock()

		Convey("The stored pressure is a continuous trajectory, not binary ±1", func() {
			interior := 0

			for _, value := range pressures.Ordered() {
				So(value, ShouldBeBetween, -1.0000001, 1.0000001)

				if value > -0.999 && value < 0.999 {
					interior++
				}
			}

			So(interior, ShouldBeGreaterThan, 0) // off the ±1 rails the binary input was stuck on
		})

		Convey("pressureFade reads a continuous value, not the degenerate constant 2", func() {
			fade := pressureFade(pressures, 1)

			So(fade, ShouldBeGreaterThan, 0)
			So(fade, ShouldNotAlmostEqual, 2.0, 0.0001) // the binary-input artifact
		})
	})
}

func TestDepthTrend(t *testing.T) {
	Convey("Given shrinking depth samples", t, func() {
		samples := ring.NewFloatRing(exitHistoryCap)
		for _, value := range []float64{10, 10, 10, 10, 8, 6} {
			samples.Push(value)
		}

		Convey("It should report positive thinning trend", func() {
			So(depthTrend(samples), ShouldBeGreaterThan, 0)
		})
	})
}
