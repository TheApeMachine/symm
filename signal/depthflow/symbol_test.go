package depthflow

import (
	"strconv"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
)

type symbolBookFixture struct {
	symbol string
}

func (fixture symbolBookFixture) snapshot(
	bidPrice, bidQty, askPrice, askQty float64,
) market.Book {
	bids := []market.BookLevel{{
		Price:    bidPrice,
		Qty:      bidQty,
		PriceRaw: strconv.FormatFloat(bidPrice, 'f', -1, 64),
		QtyRaw:   strconv.FormatFloat(bidQty, 'f', -1, 64),
	}}
	asks := []market.BookLevel{{
		Price:    askPrice,
		Qty:      askQty,
		PriceRaw: strconv.FormatFloat(askPrice, 'f', -1, 64),
		QtyRaw:   strconv.FormatFloat(askQty, 'f', -1, 64),
	}}

	update := market.Book{
		Symbol: fixture.symbol,
		Bids:   bids,
		Asks:   asks,
	}
	update.Checksum = update.ComputedChecksum()
	update.SetEnvelopeType(market.BookSnapshot)

	return update
}

func TestDepthSymbolRejectsDeltaBeforeSnapshot(t *testing.T) {
	Convey("Given a depthflow symbol fed a delta before any snapshot", t, func() {
		symbol := "ETH/EUR"
		viper.Set("market.book_depth_levels", 10)
		state, err := NewDepthSymbol(symbol)
		So(err, ShouldBeNil)
		fixture := symbolBookFixture{symbol: symbol}

		delta := fixture.snapshot(99, 8, 101, 4)
		delta.SetEnvelopeType(market.BookUpdate)
		state.ApplyBook(delta)

		Convey("It should not treat the book as ready", func() {
			So(state.HasBook(), ShouldBeFalse)
		})

		Convey("It should not emit a book-derived measurement", func() {
			state.FeedTicker(market.TickerUpdate{Symbol: symbol, Last: 100, Bid: 99, Ask: 101})
			measurement, _, err := state.Measure()

			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, perspectives.SourceNone)
		})
	})
}

func TestDepthSymbolMeasureSkipsDivergedBook(t *testing.T) {
	Convey("Given a depthflow symbol with a verified book", t, func() {
		symbol := "ETH/EUR"
		viper.Set("market.book_depth_levels", 10)
		state, err := NewDepthSymbol(symbol)
		So(err, ShouldBeNil)
		fixture := symbolBookFixture{symbol: symbol}

		state.ApplyBook(fixture.snapshot(99, 8, 101, 4))
		state.FeedTicker(market.TickerUpdate{Symbol: symbol, Last: 100, Bid: 99, Ask: 101})

		measurement, _, err := state.Measure()

		Convey("It should publish a book-derived measurement", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldNotBeEmpty)
		})

		Convey("When the maintained book diverges from the exchange checksum", func() {
			badDelta := fixture.snapshot(98, 8, 101, 4)
			badDelta.SetEnvelopeType("update")
			badDelta.Checksum = 1
			state.ApplyBook(badDelta)

			measurement, _, err := state.Measure()

			Convey("It should suppress book-derived emission", func() {
				So(err, ShouldBeNil)
				So(measurement.Source, ShouldEqual, perspectives.SourceNone)
			})
		})
	})
}

func BenchmarkDepthSymbolMeasure(b *testing.B) {
	symbol := "ETH/EUR"
	viper.Set("market.book_depth_levels", 10)
	state, err := NewDepthSymbol(symbol)

	if err != nil {
		b.Fatal(err)
	}
	fixture := symbolBookFixture{symbol: symbol}

	state.ApplyBook(fixture.snapshot(99, 8, 101, 4))
	state.FeedTicker(market.TickerUpdate{Symbol: symbol, Last: 100, Bid: 99, Ask: 101})

	b.ReportAllocs()

	for b.Loop() {
		_, _, _ = state.Measure()
	}
}
