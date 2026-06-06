package toxicity

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
)

func TestTrackerApplyOrderToxicCancel(t *testing.T) {
	Convey("Given a large near-touch cancel", t, func() {
		tracker := NewTracker()
		now := time.Now()
		symbol := "TEST/TOXIC"
		price := 100.0

		tracker.ObserveMid(symbol, market.Pair{}, price)
		state := tracker.stateLocked(symbol, market.Pair{})
		state.bidTotal = 100

		tracker.ApplyOrder(symbol, market.Pair{}, "add", "order-1", SideBid, price, 15, now, now)
		tracker.ApplyOrder(symbol, market.Pair{}, "delete", "order-1", SideBid, price, 15, now, now)

		Convey("It should flag the price level as toxic", func() {
			So(tracker.IsToxic(symbol, price, now), ShouldBeTrue)
		})
	})
}

func TestTrackerFillToCancelThreshold(t *testing.T) {
	Convey("Given a tracker without cached ratio", t, func() {
		tracker := NewTracker()

		Convey("It should lazily load threshold from viper", func() {
			So(tracker.fillToCancelThreshold(), ShouldBeGreaterThanOrEqualTo, 0)
		})
	})
}

func BenchmarkTrackerApplyOrder(b *testing.B) {
	tracker := NewTracker()
	now := time.Now()
	symbol := "BTC/EUR"

	tracker.ObserveMid(symbol, market.Pair{}, 100)
	state := tracker.stateLocked(symbol, market.Pair{})
	state.bidTotal = 100

	for b.Loop() {
		tracker.ApplyOrder(symbol, market.Pair{}, "add", "order-1", SideBid, 100, 15, now, now)
		tracker.ApplyOrder(symbol, market.Pair{}, "delete", "order-1", SideBid, 100, 15, now, now)
	}
}
