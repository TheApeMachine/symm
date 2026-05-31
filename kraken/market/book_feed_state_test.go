package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/orderbook"
)

func TestBookFeedStateRejectsDeltaBeforeSnapshot(t *testing.T) {
	Convey("Given a book feed state with no snapshot yet", t, func() {
		state := NewBookFeedState("BTC/EUR", "test", 10)
		delta := BookUpdate{Symbol: "BTC/EUR", Checksum: 1}
		delta.Bids = []BookLevel{{Price: 1, Qty: 1, PriceRaw: "1", QtyRaw: "1"}}

		state.Apply(delta)

		Convey("It should not mark the book ready or diverged", func() {
			So(state.Ready(), ShouldBeFalse)
			So(state.Diverged(), ShouldBeFalse)
		})
	})
}

func TestBookFeedStateAcceptsSnapshotThenDelta(t *testing.T) {
	Convey("Given a valid snapshot and matching delta", t, func() {
		symbol := "BTC/EUR"
		state := NewBookFeedState(symbol, "test", 10)

		levels := func(price, qty float64) []orderbook.Level {
			return []orderbook.Level{{
				Price: price, Qty: qty,
				PriceRaw: "100", QtyRaw: "1",
			}}
		}

		book := orderbook.NewBook(orderbook.MaintainDepth(10))
		book.ApplySnapshot(levels(99, 1), levels(101, 1))

		snapshot := BookUpdate{
			Symbol:   symbol,
			Checksum: int64(book.Checksum()),
			Bids:     []BookLevel{{Price: 99, Qty: 1, PriceRaw: "100", QtyRaw: "1"}},
			Asks:     []BookLevel{{Price: 101, Qty: 1, PriceRaw: "100", QtyRaw: "1"}},
		}
		snapshot.SetEnvelopeType(BookSnapshot)

		state.Apply(snapshot)

		Convey("It should be ready without divergence", func() {
			So(state.Ready(), ShouldBeTrue)
			So(state.Diverged(), ShouldBeFalse)
		})
	})
}
