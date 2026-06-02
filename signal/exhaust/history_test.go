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
