package hindsight

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStateVersionMonotonicTest(t *testing.T) {
	Convey("Given a shared resident state cell", t, func() {
		var version uint64

		Convey("Advancing it from two workloads keeps the version monotonic", func() {
			version++
			fromTrade := version

			version++
			fromTicker := version

			So(fromTicker, ShouldBeGreaterThan, fromTrade)
			So(StateVersion{
				Component: "strategy.planner",
				Key:       "BTC",
				Version:   fromTrade,
			}, ShouldNotResemble, StateVersion{
				Component: "strategy.planner",
				Key:       "BTC",
				Version:   fromTicker,
			})
		})
	})
}
