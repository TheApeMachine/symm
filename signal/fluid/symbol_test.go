package fluid

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
)

type symbolBookFixture struct {
	symbol string
}

func (fixture symbolBookFixture) snapshot(
	bidPrice, bidQty, askPrice, askQty float64,
) market.Book {
	bids := []market.BookLevel{{Price: bidPrice, Qty: bidQty}}
	asks := []market.BookLevel{{Price: askPrice, Qty: askQty}}

	update := market.Book{
		Symbol: fixture.symbol,
		Bids:   bids,
		Asks:   asks,
	}
	update.Checksum = update.ComputedChecksum()
	update.SetEnvelopeType(market.BookSnapshot)

	return update
}

func TestFluidSymbolRejectsDeltaBeforeSnapshot(t *testing.T) {
	Convey("Given a fluid symbol fed a delta before any snapshot", t, func() {
		symbol := "ETH/EUR"
		state := NewFluidSymbol(symbol)
		fixture := symbolBookFixture{symbol: symbol}

		delta := fixture.snapshot(99, 10, 101, 6)
		delta.SetEnvelopeType("update")
		state.FeedBook(delta)

		Convey("It should not treat the book as ready", func() {
			So(state.HasBook(), ShouldBeFalse)
		})

		Convey("It should report Measure as not ready", func() {
			_, ok := state.Measure()

			So(ok, ShouldBeFalse)
		})
	})
}

func TestFluidSymbolMeasureSkipsDivergedBook(t *testing.T) {
	Convey("Given a fluid symbol with a verified book", t, func() {
		symbol := "ETH/EUR"
		state := NewFluidSymbol(symbol)
		fixture := symbolBookFixture{symbol: symbol}

		state.FeedTicker(market.TickerUpdate{
			Symbol: symbol, Last: 100, Bid: 99, Ask: 101, Volume: 1000,
		})
		state.FeedBook(fixture.snapshot(99, 10, 101, 6))

		_, ok := state.Measure()

		Convey("It should publish a field reading", func() {
			So(ok, ShouldBeTrue)
		})

		Convey("When the maintained book diverges from the exchange checksum", func() {
			badDelta := fixture.snapshot(98, 10, 101, 6)
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
	fixture := symbolBookFixture{symbol: symbol}

	state.FeedTicker(market.TickerUpdate{
		Symbol: symbol, Last: 100, Bid: 99, Ask: 101, Volume: 1000,
	})
	state.FeedBook(fixture.snapshot(99, 10, 101, 6))

	b.ReportAllocs()

	for b.Loop() {
		_, _ = state.Measure()
	}
}
