package toxicity

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestTrackerApplyBookFramePreservesLevelAge(t *testing.T) {
	Convey("Given a book level held across frames", t, func() {
		tracker := NewTracker()
		pair := krakenmarket.Pair{TickSize: "0.01"}
		symbol := "BTC/USD"
		startAt := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

		tracker.ObserveMid(symbol, pair, 100)

		initialBook := &krakenmarket.BookUpdate{
			Asks: []krakenmarket.BookLevel{{Price: 100.01, Qty: 10}},
		}
		tracker.ApplyBookFrame(symbol, pair, initialBook, startAt)

		heldAt := startAt.Add(15 * time.Second)
		heldBook := &krakenmarket.BookUpdate{
			Asks: []krakenmarket.BookLevel{{Price: 100.01, Qty: 10}},
		}
		tracker.ApplyBookFrame(symbol, pair, heldBook, heldAt)

		removedAt := heldAt.Add(time.Millisecond)
		removedBook := &krakenmarket.BookUpdate{
			Asks: []krakenmarket.BookLevel{},
		}
		tracker.ApplyBookFrame(symbol, pair, removedBook, removedAt)

		snapshot, _, ok := tracker.Snapshot(symbol, removedAt)

		Convey("It should record cancellations from aged levels", func() {
			So(ok, ShouldBeTrue)
			So(snapshot.cancelAsk, ShouldAlmostEqual, 0.5, 1e-9)
		})

		Convey("It should not flag stale near-touch cancels as toxic bluffs", func() {
			So(tracker.IsToxic(symbol, 100.01, removedAt), ShouldBeFalse)
		})
	})
}

func TestTrackerApplyBookDeltaPreservesUntouchedLevels(t *testing.T) {
	Convey("Given a snapshot followed by a partial update", t, func() {
		tracker := NewTracker()
		pair := krakenmarket.Pair{TickSize: "0.01"}
		symbol := "BTC/USD"
		startAt := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

		tracker.ObserveMid(symbol, pair, 100)

		snapshot := &krakenmarket.BookUpdate{
			Type: "snapshot",
			Bids: []krakenmarket.BookLevel{{Price: 99.99, Qty: 10}},
			Asks: []krakenmarket.BookLevel{{Price: 100.01, Qty: 10}},
		}
		tracker.ApplyBookFrame(symbol, pair, snapshot, startAt)

		deltaAt := startAt.Add(time.Millisecond)
		delta := &krakenmarket.BookUpdate{
			Type: "update",
			Asks: []krakenmarket.BookLevel{{Price: 100.01, Qty: 5}},
		}
		tracker.ApplyBookDelta(symbol, pair, delta, deltaAt)

		bookSnapshot, _, ok := tracker.Snapshot(symbol, deltaAt)

		Convey("It should keep bid depth from the snapshot", func() {
			So(ok, ShouldBeTrue)
			So(bookSnapshot.bidDepth, ShouldEqual, 10)
			So(bookSnapshot.askDepth, ShouldEqual, 5)
		})
	})
}

func TestTrackerApplyBookFrameDetectsPartialDepletion(t *testing.T) {
	Convey("Given a repeated book frame with reduced quantity", t, func() {
		tracker := NewTracker()
		pair := krakenmarket.Pair{TickSize: "0.01"}
		symbol := "ETH/EUR"
		startAt := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

		tracker.ObserveMid(symbol, pair, 100)

		fullBook := &krakenmarket.BookUpdate{
			Asks: []krakenmarket.BookLevel{{Price: 100.01, Qty: 10}},
		}
		tracker.ApplyBookFrame(symbol, pair, fullBook, startAt)

		reducedAt := startAt.Add(time.Second)
		reducedBook := &krakenmarket.BookUpdate{
			Asks: []krakenmarket.BookLevel{{Price: 100.01, Qty: 5}},
		}
		tracker.ApplyBookFrame(symbol, pair, reducedBook, reducedAt)

		snapshot, _, ok := tracker.Snapshot(symbol, reducedAt)

		Convey("It should attribute the delta to cancellation flow", func() {
			So(ok, ShouldBeTrue)
			So(snapshot.cancelAsk, ShouldAlmostEqual, 0.25, 1e-9)
			So(snapshot.askDepth, ShouldEqual, 5)
		})
	})
}
