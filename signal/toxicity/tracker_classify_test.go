package toxicity

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
)

func TestClassifyRemovalLocked(t *testing.T) {
	Convey("Given a large near-touch cancel", t, func() {
		tracker := newTestTracker(t)
		now := time.Now()
		symbol := "BTC/EUR"
		price := 100.0

		tracker.ObserveMid(symbol, market.Pair{}, price)

		state := tracker.stateLocked(symbol, market.Pair{})
		state.bidTotal = 100

		tracker.ApplyOrder(symbol, market.Pair{}, "add", "order-1", SideBid, price, 15, now, now)
		tracker.ApplyOrder(symbol, market.Pair{}, "delete", "order-1", SideBid, price, 15, now, now)

		Convey("It should flag toxic near-touch cancels", func() {
			So(tracker.IsToxic(symbol, price, now), ShouldBeTrue)
		})
	})
}
