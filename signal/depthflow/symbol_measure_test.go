package depthflow

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

func TestDepthSymbolMeasureSkipsDivergedBook(t *testing.T) {
	Convey("Given a depthflow symbol with a verified book", t, func() {
		symbol := "ETH/EUR"
		state := NewDepthSymbol(symbol)

		state.ApplyBook(snapshotWithChecksum(symbol, 99, 8, 101, 4))
		state.FeedTicker(market.TickerUpdate{Symbol: symbol, Last: 100, Bid: 99, Ask: 101})

		_, ok := state.Measure()

		Convey("It should publish a book-derived measurement", func() {
			So(ok, ShouldBeTrue)
		})

		Convey("When the maintained book diverges from the exchange checksum", func() {
			badDelta := snapshotWithChecksum(symbol, 98, 8, 101, 4)
			badDelta.SetEnvelopeType("update")
			badDelta.Checksum = 1
			state.ApplyBook(badDelta)

			_, ok := state.Measure()

			Convey("It should suppress book-derived emission", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func BenchmarkDepthSymbolMeasure(b *testing.B) {
	symbol := "ETH/EUR"
	state := NewDepthSymbol(symbol)
	state.ApplyBook(snapshotWithChecksum(symbol, 99, 8, 101, 4))
	state.FeedTicker(market.TickerUpdate{Symbol: symbol, Last: 100, Bid: 99, Ask: 101})

	b.ReportAllocs()

	for b.Loop() {
		_, _ = state.Measure()
	}
}
