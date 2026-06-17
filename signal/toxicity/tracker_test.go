package toxicity

import (
	"context"
	"sync"
	"sync/atomic"
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
		state := tracker.symbolState(symbol)
		alpha := state.timing.FlowSmoothingAlpha(
			state.timing.MatchWindow(state.tradeSpan()),
			state.tradeSpan(),
			len(state.trades),
		)

		Convey("It should record cancellations from aged levels", func() {
			So(ok, ShouldBeTrue)
			So(snapshot.cancelAsk, ShouldAlmostEqual, alpha*10, 1e-9)
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
		state := tracker.symbolState(symbol)
		alpha := state.timing.FlowSmoothingAlpha(
			state.timing.MatchWindow(state.tradeSpan()),
			state.tradeSpan(),
			len(state.trades),
		)

		Convey("It should attribute the delta to cancellation flow", func() {
			So(ok, ShouldBeTrue)
			So(snapshot.cancelAsk, ShouldAlmostEqual, alpha*5, 1e-9)
			So(snapshot.askDepth, ShouldEqual, 5)
		})
	})
}

func TestConcurrentTrackerApplyBookDelta(t *testing.T) {
	Convey("Given concurrent book deltas on a live tracker", t, func() {
		tracker := NewConcurrentTracker(context.Background())
		defer tracker.Close()

		pair := krakenmarket.Pair{TickSize: "0.01"}
		symbol := "BTC/USD"
		startAt := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

		tracker.ObserveMid(symbol, pair, 100)
		tracker.ApplyBookFrame(symbol, pair, &krakenmarket.BookUpdate{
			Type: "snapshot",
			Bids: []krakenmarket.BookLevel{{Price: 99.99, Qty: 10}},
			Asks: []krakenmarket.BookLevel{{Price: 100.01, Qty: 10}},
		}, startAt)

		var waitGroup sync.WaitGroup

		for workerIndex := range 16 {
			waitGroup.Add(1)

			go func(index int) {
				defer waitGroup.Done()

				price := 100.01 + float64(index)*0.01
				tracker.ApplyBookDelta(symbol, pair, &krakenmarket.BookUpdate{
					Type: "update",
					Asks: []krakenmarket.BookLevel{{Price: price, Qty: float64(index + 1)}},
				}, startAt.Add(time.Duration(index)*time.Millisecond))
				tracker.Snapshot(symbol, startAt.Add(time.Second))
			}(workerIndex)
		}

		waitGroup.Wait()

		Convey("It should finish without map races and preserve ask depth", func() {
			snapshot, _, ok := tracker.Snapshot(symbol, startAt.Add(time.Second))

			So(ok, ShouldBeTrue)
			So(snapshot.askDepth, ShouldBeGreaterThan, 0)
		})
	})
}

func TestTrackerMeasureFeaturesUnderConcurrentReads(testingTB *testing.T) {
	Convey("Given concurrent measure reads on a live tracker", testingTB, func() {
		tracker := NewConcurrentTracker(context.Background())
		defer tracker.Close()

		pair := krakenmarket.Pair{TickSize: "0.01"}
		symbol := "BTC/USD"
		startAt := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

		tracker.ObserveMid(symbol, pair, 100)
		tracker.ApplyBookFrame(symbol, pair, &krakenmarket.BookUpdate{
			Type: "snapshot",
			Bids: []krakenmarket.BookLevel{{Price: 99.99, Qty: 10}},
			Asks: []krakenmarket.BookLevel{{Price: 100.01, Qty: 10}},
		}, startAt)

		done := make(chan struct{})
		var waitGroup sync.WaitGroup
		var mismatch atomic.Bool

		for range 32 {
			waitGroup.Add(1)

			go func() {
				defer waitGroup.Done()

				for range 32 {
					features, ok := tracker.measureFeatures(symbol)

					if !ok {
						continue
					}

					if features.lastPrice != 100 {
						mismatch.Store(true)
					}
				}
			}()
		}

		go func() {
			waitGroup.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			testingTB.Fatal("timed out waiting for concurrent measure reads")
		}

		Convey("It should keep last price aligned with observed mid", func() {
			So(mismatch.Load(), ShouldBeFalse)
		})
	})
}

func TestDefaultTracker(testingTB *testing.T) {
	Convey("Given the process-wide tracker", testingTB, func() {
		initial := defaultTracker.Load()
		ResetDefault()
		swapped := defaultTracker.Load()

		Convey("It should swap the default instance on reset", func() {
			So(initial, ShouldNotBeNil)
			So(swapped, ShouldNotBeNil)
			So(swapped == initial, ShouldBeFalse)
		})
	})
}

func TestIsToxicHelper(testingTB *testing.T) {
	Convey("Given a toxic cancel on the default tracker", testingTB, func() {
		ResetDefault()
		tracker := defaultTracker.Load()
		pair := krakenmarket.Pair{TickSize: "0.01"}
		symbol := "ZZZ/ISOLATED"
		startAt := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

		tracker.ObserveMid(symbol, pair, 100)
		tracker.ApplyBookFrame(symbol, pair, &krakenmarket.BookUpdate{
			Asks: []krakenmarket.BookLevel{{Price: 100.01, Qty: 10}},
		}, startAt)

		removedAt := startAt.Add(15 * time.Second)
		tracker.ApplyBookFrame(symbol, pair, &krakenmarket.BookUpdate{
			Asks: []krakenmarket.BookLevel{},
		}, removedAt)

		Convey("It should delegate IsToxic to the active tracker", func() {
			So(IsToxic(symbol, 100.01, removedAt), ShouldBeFalse)
		})
	})
}

func TestNearTouchToxic(testingTB *testing.T) {
	Convey("Given a near-touch toxic flag on the shared tracker", testingTB, func() {
		ResetDefault()
		tracker := defaultTracker.Load()
		now := time.Now()
		symbol := "BTC/EUR"
		price := 100.0

		tracker.ObserveMid(symbol, krakenmarket.Pair{}, price)
		state := tracker.stateLocked(symbol, krakenmarket.Pair{})
		state.flow.BidDepth = 100

		tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "add", "order-1", SideBid, price, 15, now, now)
		tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "delete", "order-1", SideBid, price, 15, now, now)

		Convey("It should report near-touch toxicity for that symbol", func() {
			So(NearTouchToxic(symbol, now), ShouldBeTrue)
			So(NearTouchToxic("ETH/EUR", now), ShouldBeFalse)
		})
	})
}

func TestPriceKey(testingTB *testing.T) {
	Convey("Given a pair with tick size", testingTB, func() {
		state := &symbolState{pair: krakenmarket.Pair{TickSize: "0.1"}}

		Convey("It should round prices to tick boundaries", func() {
			So(priceKey(state, 100.01), ShouldEqual, priceKey(state, 100.04))
			So(priceFromKey(state, priceKey(state, 100.01)), ShouldAlmostEqual, 100.0, 1e-9)
		})
	})
	Convey("Given a pair without tick size", testingTB, func() {
		state := newSymbolState(krakenmarket.Pair{})
		state.mid = 100

		for _, step := range []float64{0.0001, 0.00012, 0.00011} {
			state.priceIncrements.Observe(step)
		}

		Convey("It should discretize from observed price increments", func() {
			key := priceKey(state, 100.000000001)
			So(priceFromKey(state, key), ShouldAlmostEqual, 100.0, 1e-4)
		})
	})
}

func TestIsToxicPriceKeyLookup(testingTB *testing.T) {
	Convey("Given a toxic level stored at a rounded price", testingTB, func() {
		tracker := NewTracker()
		symbol := "ETH/EUR"
		now := time.Now()
		pair := krakenmarket.Pair{TickSize: "0.01"}

		state := tracker.stateLocked(symbol, pair)
		matchWindow := state.timing.MatchWindow(state.tradeSpan())
		state.toxic.Flag(
			priceKey(state, 100.0),
			0,
			1,
			now.Add(state.timing.Cooldown(matchWindow)),
		)

		Convey("It should match a slightly perturbed lookup price", func() {
			So(tracker.IsToxic(symbol, 100.0000004, now), ShouldBeTrue)
		})
		Convey("It should match one tick away", func() {
			So(tracker.IsToxic(symbol, 100.01, now), ShouldBeTrue)
		})
	})
}

func TestTrackerApplyOrderToxicCancel(testingTB *testing.T) {
	Convey("Given a large near-touch cancel", testingTB, func() {
		tracker := NewTracker()
		now := time.Now()
		symbol := "TEST/TOXIC"
		price := 100.0

		tracker.ObserveMid(symbol, krakenmarket.Pair{}, price)
		state := tracker.stateLocked(symbol, krakenmarket.Pair{})
		state.flow.BidDepth = 100

		tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "add", "order-1", SideBid, price, 15, now, now)
		tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "delete", "order-1", SideBid, price, 15, now, now)

		Convey("It should flag the price level as toxic", func() {
			So(tracker.IsToxic(symbol, price, now), ShouldBeTrue)
		})
	})
}

func TestTrackerFlashChurnFlagsNearTouchLevel(testingTB *testing.T) {
	Convey("Given rapid near-touch add/delete churn without fills", testingTB, func() {
		tracker := NewTracker()
		now := time.Now()
		symbol := "BTC/EUR"
		price := 100.0

		tracker.ObserveMid(symbol, krakenmarket.Pair{}, price)
		state := tracker.stateLocked(symbol, krakenmarket.Pair{})
		state.flow.BidDepth = 100

		tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "add", "order-1", SideBid, price, 15, now, now)
		tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "delete", "order-1", SideBid, price, 15, now, now)

		Convey("It should flag the price level as toxic", func() {
			So(tracker.IsToxic(symbol, price, now), ShouldBeTrue)
		})
	})
}

func TestTrackerFillToCancelThreshold(testingTB *testing.T) {
	Convey("Given a tracker without cached ratio", testingTB, func() {
		tracker := NewTracker()

		Convey("It should derive threshold from symbol flow", func() {
			So(tracker.fillToCancelThreshold(), ShouldBeGreaterThanOrEqualTo, 0)
		})
	})
}

func TestTrackerBookSideDepth(testingTB *testing.T) {
	Convey("Given mid-price observations", testingTB, func() {
		tracker := NewTracker()
		now := time.Now()

		tracker.ObserveMid("BTC/EUR", krakenmarket.Pair{}, 100)
		tracker.ObserveLast("BTC/EUR", krakenmarket.Pair{}, 101)

		Convey("It should retain symbol state", func() {
			So(tracker.IsToxic("BTC/EUR", 100, now), ShouldBeFalse)
		})
	})
}

func TestTrackerLevel3Churn(testingTB *testing.T) {
	Convey("Given a level3 update with add/delete events", testingTB, func() {
		ResetDefault()
		tracker := NewTracker()

		now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		tracker.ObserveMid("BTC/EUR", krakenmarket.Pair{}, 100)
		state := tracker.stateLocked("BTC/EUR", krakenmarket.Pair{})
		state.flow.BidDepth = 100

		tracker.ApplyOrder("BTC/EUR", krakenmarket.Pair{}, "add", "l3-2", SideBid, 100, 15, now, now)
		tracker.ApplyOrder("BTC/EUR", krakenmarket.Pair{}, "delete", "l3-2", SideBid, 100, 15, now, now)

		Convey("It should classify per-order churn as toxic", func() {
			So(tracker.IsToxic("BTC/EUR", 100, now), ShouldBeTrue)
		})
	})
}

func BenchmarkTrackerApplyOrder(b *testing.B) {
	tracker := NewTracker()
	now := time.Now()
	symbol := "BTC/EUR"
	price := 100.0

	tracker.ObserveMid(symbol, krakenmarket.Pair{}, price)
	state := tracker.stateLocked(symbol, krakenmarket.Pair{})
	state.flow.BidDepth = 100

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "add", "order-1", SideBid, price, 15, now, now)
		tracker.ApplyOrder(symbol, krakenmarket.Pair{}, "delete", "order-1", SideBid, price, 15, now, now)
	}
}
