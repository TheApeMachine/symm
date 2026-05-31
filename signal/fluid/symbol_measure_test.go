package fluid

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/orderbook"
)

func snapshotWithChecksum(
	symbol string,
	bidPrice, bidQty, askPrice, askQty float64,
) market.BookUpdate {
	levels := func(price, qty float64) []orderbook.Level {
		rawPrice := fmt.Sprintf("%g", price)
		rawQty := fmt.Sprintf("%g", qty)

		return []orderbook.Level{{
			Price: price, Qty: qty, PriceRaw: rawPrice, QtyRaw: rawQty,
		}}
	}

	book := orderbook.NewBook(orderbook.MaintainDepth(10))
	book.ApplySnapshot(levels(bidPrice, bidQty), levels(askPrice, askQty))

	update := market.BookUpdate{
		Symbol:   symbol,
		Checksum: int64(book.Checksum()),
		Bids: []market.BookLevel{{
			Price: bidPrice, Qty: bidQty,
			PriceRaw: fmt.Sprintf("%g", bidPrice), QtyRaw: fmt.Sprintf("%g", bidQty),
		}},
		Asks: []market.BookLevel{{
			Price: askPrice, Qty: askQty,
			PriceRaw: fmt.Sprintf("%g", askPrice), QtyRaw: fmt.Sprintf("%g", askQty),
		}},
	}
	update.SetEnvelopeType(market.BookSnapshot)

	return update
}

func TestFluidSymbolRejectsDeltaBeforeSnapshot(t *testing.T) {
	Convey("Given a fluid symbol fed a delta before any snapshot", t, func() {
		symbol := "ETH/EUR"
		state := NewFluidSymbol(symbol)

		delta := snapshotWithChecksum(symbol, 99, 10, 101, 6)
		delta.SetEnvelopeType("update")
		state.FeedBook(delta)

		Convey("It should not treat the book as ready", func() {
			So(state.HasBook(), ShouldBeFalse)
		})

	})
}

func TestFluidSymbolMeasureSkipsDivergedBook(t *testing.T) {
	Convey("Given a fluid symbol with a verified book", t, func() {
		symbol := "ETH/EUR"
		state := NewFluidSymbol(symbol)

		state.FeedTicker(market.TickerUpdate{
			Symbol: symbol, Last: 100, Bid: 99, Ask: 101, Volume: 1000,
		})
		state.FeedBook(snapshotWithChecksum(symbol, 99, 10, 101, 6))

		_, ok := state.Measure()

		Convey("It should publish a field reading", func() {
			So(ok, ShouldBeTrue)
		})

		Convey("When the maintained book diverges from the exchange checksum", func() {
			badDelta := snapshotWithChecksum(symbol, 98, 10, 101, 6)
			badDelta.SetEnvelopeType("update")
			badDelta.Checksum = 1
			state.FeedBook(badDelta)

			_, ok := state.Measure()

			Convey("It should suppress field emission", func() {
				So(ok, ShouldBeFalse)
			})

			Convey("It should suppress dashboard rows", func() {
				So(state.Row(), ShouldBeNil)
			})
		})
	})
}

func BenchmarkFluidSymbolMeasure(b *testing.B) {
	symbol := "ETH/EUR"
	state := NewFluidSymbol(symbol)
	state.FeedTicker(market.TickerUpdate{
		Symbol: symbol, Last: 100, Bid: 99, Ask: 101, Volume: 1000,
	})
	state.FeedBook(snapshotWithChecksum(symbol, 99, 10, 101, 6))

	b.ReportAllocs()

	for b.Loop() {
		_, _ = state.Measure()
	}
}
