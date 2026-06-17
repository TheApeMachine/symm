package toxicity

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestCloneSymbolState(testingTB *testing.T) {
	Convey("Given a populated symbol state", testingTB, func() {
		pair := krakenmarket.Pair{TickSize: "0.01"}
		original := newSymbolState(pair)
		original.mid = 100
		original.lastPrice = 101
		original.flow.CancelBid = 0.4
		original.orders["order-1"] = &orderState{price: 99.99, qty: 2}
		original.levels[l2Key{side: SideAsk, price: 100.01}] = &l2Level{qty: 10, firstSeen: time.Now()}

		clone := cloneSymbolState(original)

		Convey("It should copy scalar and map fields", func() {
			So(clone.mid, ShouldEqual, original.mid)
			So(clone.lastPrice, ShouldEqual, original.lastPrice)
			So(clone.flow.CancelBid, ShouldEqual, original.flow.CancelBid)
			So(clone.orders["order-1"].qty, ShouldEqual, 2)

			levelKey := l2Key{side: SideAsk, price: 100.01}
			So(clone.levels[levelKey].qty, ShouldEqual, 10)
		})

		Convey("It should isolate mutations from the source", func() {
			clone.orders["order-1"].qty = 5
			levelKey := l2Key{side: SideAsk, price: 100.01}
			clone.levels[levelKey].qty = 1

			So(original.orders["order-1"].qty, ShouldEqual, 2)
			So(original.levels[levelKey].qty, ShouldEqual, 10)
		})
	})
}

func TestTrackerMutateStateCopyOnWrite(testingTB *testing.T) {
	Convey("Given a tracker with one symbol", testingTB, func() {
		tracker := NewTracker()
		pair := krakenmarket.Pair{TickSize: "0.01"}
		symbol := "BTC/USD"
		eventAt := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

		tracker.ObserveMid(symbol, pair, 100)

		tracker.mutateState(symbol, pair, eventAt, false, "test", func(state *symbolState) {
			state.lastPrice = 101
		})

		firstSnapshot := tracker.symbolState(symbol).lastPrice

		tracker.mutateState(symbol, pair, eventAt.Add(time.Millisecond), false, "test", func(state *symbolState) {
			state.lastPrice = 102
		})

		secondSnapshot := tracker.symbolState(symbol).lastPrice

		Convey("It should publish successive immutable snapshots", func() {
			So(firstSnapshot, ShouldEqual, 101)
			So(secondSnapshot, ShouldEqual, 102)
		})
	})
}

func TestCloneSymbolStateNil(testingTB *testing.T) {
	Convey("Given a nil symbol state", testingTB, func() {
		Convey("It should return nil", func() {
			So(cloneSymbolState(nil), ShouldBeNil)
		})
	})
}
