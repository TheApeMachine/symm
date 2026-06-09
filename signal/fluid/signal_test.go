package fluid

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

type symbolBookFixture struct {
	symbol string
}

func (fixture symbolBookFixture) snapshot(
	bidPrice, bidQty, askPrice, askQty float64,
) krakenmarket.Book {
	bids := []krakenmarket.BookLevel{{Price: bidPrice, Qty: bidQty}}
	asks := []krakenmarket.BookLevel{{Price: askPrice, Qty: askQty}}

	update := krakenmarket.Book{
		Symbol: fixture.symbol,
		Bids:   bids,
		Asks:   asks,
	}
	update.Checksum = update.ComputedChecksum()
	update.SetEnvelopeType(krakenmarket.BookSnapshot)

	return update
}

func TestFluidSymbolIgnoresFluxBeforeVolumeClock(t *testing.T) {
	Convey("Given book and trade updates before the volume clock is seeded", t, func() {
		symbol := "ETH/EUR"
		viper.Set("market.book_depth_levels", 10)
		viper.Set("signals.volume_clock_bars_per_day", 288)
		state, err := NewFluidSymbol(symbol)
		So(err, ShouldBeNil)
		fixture := symbolBookFixture{symbol: symbol}

		So(state.FeedBook(fixture.snapshot(99, 10, 101, 6)), ShouldBeNil)
		So(state.FeedTradeSide(time.Now(), 1, "buy"), ShouldBeNil)

		Convey("It should wait for ticker volume before folding flux", func() {
			So(state.flux.hasTarget(), ShouldBeFalse)
		})
	})
}

func TestFluidSymbolRejectsDeltaBeforeSnapshot(t *testing.T) {
	Convey("Given a fluid symbol fed a delta before any snapshot", t, func() {
		symbol := "ETH/EUR"
		viper.Set("market.book_depth_levels", 10)
		viper.Set("signals.volume_clock_bars_per_day", 288)
		state, err := NewFluidSymbol(symbol)
		So(err, ShouldBeNil)
		fixture := symbolBookFixture{symbol: symbol}

		delta := fixture.snapshot(99, 10, 101, 6)
		delta.SetEnvelopeType("update")
		So(state.FeedBook(delta), ShouldBeNil)

		Convey("It should not treat the book as ready", func() {
			So(state.HasBook(), ShouldBeFalse)
		})

		Convey("It should report Reading as not ready", func() {
			_, ok := state.Reading()

			So(ok, ShouldBeFalse)
		})
	})
}

func TestFluidSymbolMeasureSkipsDivergedBook(t *testing.T) {
	Convey("Given a fluid symbol with a verified book", t, func() {
		symbol := "ETH/EUR"
		viper.Set("market.book_depth_levels", 10)
		viper.Set("signals.volume_clock_bars_per_day", 288)
		state, err := NewFluidSymbol(symbol)
		So(err, ShouldBeNil)
		fixture := symbolBookFixture{symbol: symbol}

		So(state.FeedTicker(krakenmarket.TickerUpdate{
			Symbol: symbol, Last: 100, Bid: 99, Ask: 101, Volume: 1000,
		}), ShouldBeNil)
		So(state.FeedBook(fixture.snapshot(99, 10, 101, 6)), ShouldBeNil)

		_, ok := state.Reading()

		Convey("It should publish a field reading", func() {
			So(ok, ShouldBeTrue)
		})

		Convey("When the maintained book diverges from the exchange checksum", func() {
			badDelta := fixture.snapshot(98, 10, 101, 6)
			badDelta.SetEnvelopeType("update")
			badDelta.Checksum = 1
			So(state.FeedBook(badDelta), ShouldBeNil)

			_, ok := state.Reading()

			Convey("It should suppress field emission", func() {
				So(ok, ShouldBeFalse)
			})

			Convey("It should suppress dashboard rows", func() {
				So(state.Row(), ShouldBeNil)
			})
		})
	})
}

func TestFluidSymbolMeasureLaminarField(t *testing.T) {
	Convey("Given a balanced book with no Reynolds activity", t, func() {
		symbol := "BTC/EUR"
		viper.Set("market.book_depth_levels", 10)
		viper.Set("signals.volume_clock_bars_per_day", 288)
		state, err := NewFluidSymbol(symbol)
		So(err, ShouldBeNil)
		fixture := symbolBookFixture{symbol: symbol}

		So(state.FeedTicker(krakenmarket.TickerUpdate{
			Symbol: symbol, Last: 100, Bid: 100, Ask: 100, Volume: 1000,
		}), ShouldBeNil)
		So(state.FeedBook(fixture.snapshot(100, 5, 100, 5)), ShouldBeNil)

		reading, ok := state.Reading()
		signal := NewSignal(symbol, logic.NewEntity(logic.EntityBook), 8, nil, 2.0, 0.5)
		category, _, _, _, _ := signal.classify(reading)

		Convey("It should still publish a laminar reading", func() {
			So(ok, ShouldBeTrue)
			So(category, ShouldEqual, logic.CategoryLaminar)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	symbol := "ETH/EUR"
	viper.Set("market.book_depth_levels", 10)
	viper.Set("signals.volume_clock_bars_per_day", 288)
	state, err := NewFluidSymbol(symbol)

	if err != nil {
		b.Fatal(err)
	}

	fixture := symbolBookFixture{symbol: symbol}

	if err := state.FeedTicker(krakenmarket.TickerUpdate{
		Symbol: symbol, Last: 100, Bid: 99, Ask: 101, Volume: 1000,
	}); err != nil {
		b.Fatal(err)
	}

	if err := state.FeedBook(fixture.snapshot(99, 10, 101, 6)); err != nil {
		b.Fatal(err)
	}

	signal := NewSignal(symbol, logic.NewEntity(logic.EntityBook), 8, nil, 2.0, 0.5)

	b.ReportAllocs()

	for b.Loop() {
		reading, ok := state.Reading()

		if ok {
			_, _ = signal.publish(reading)
		}
	}
}
