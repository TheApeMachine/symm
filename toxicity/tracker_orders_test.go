package toxicity

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
)

func TestTrackerOrdersFlow(t *testing.T) {
	Convey("Given consecutive order events", t, func() {
		tracker := NewTracker()
		now := time.Now()

		tracker.ApplyOrder("BTC/EUR", market.Pair{}, "add", "order-1", SideBid, 100, 2, now, now)
		tracker.ApplyOrder("BTC/EUR", market.Pair{}, "delete", "order-1", SideBid, 100, 2, now, now)

		Convey("It should track order flow without panic", func() {
			So(tracker, ShouldNotBeNil)
		})
	})
}
