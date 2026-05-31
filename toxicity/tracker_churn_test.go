package toxicity

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
)

func TestTrackerFlashChurnFlagsNearTouchLevel(t *testing.T) {
	convey.Convey("Given rapid near-touch add/delete churn without fills", t, func() {
		tracker := NewTracker()
		now := time.Now()
		symbol := "BTC/EUR"
		price := 100.0

		tracker.ObserveMid(symbol, market.Pair{}, price)
		state := tracker.stateLocked(symbol, market.Pair{})
		state.bidTotal = 100

		tracker.ApplyOrder(symbol, market.Pair{}, "add", "order-1", SideBid, price, 15, now, now)
		tracker.ApplyOrder(symbol, market.Pair{}, "delete", "order-1", SideBid, price, 15, now, now)

		convey.Convey("It should flag the price level as toxic", func() {
			convey.So(tracker.IsToxic(symbol, price, now), convey.ShouldBeTrue)
		})
	})
}

func BenchmarkTrackerObserveLevelChurn(b *testing.B) {
	tracker := NewTracker()
	now := time.Now()
	symbol := "BTC/EUR"

	tracker.ObserveMid(symbol, market.Pair{}, 100)
	state := tracker.stateLocked(symbol, market.Pair{})
	state.bidTotal = 100

	b.ReportAllocs()

	for b.Loop() {
		tracker.ApplyOrder(symbol, market.Pair{}, "add", "order-1", SideBid, 100, 15, now, now)
		tracker.ApplyOrder(symbol, market.Pair{}, "delete", "order-1", SideBid, 100, 15, now, now)
	}
}
