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
) krakenmarket.BookUpdate {
	bids := []krakenmarket.BookLevel{
		{Price: bidPrice, Qty: bidQty},
		{Price: bidPrice - 0.01, Qty: bidQty * 0.5},
	}
	asks := []krakenmarket.BookLevel{
		{Price: askPrice, Qty: askQty},
		{Price: askPrice + 0.01, Qty: askQty * 0.5},
	}

	return krakenmarket.BookUpdate{
		Symbol: fixture.symbol,
		Type:   "snapshot",
		Bids:   bids,
		Asks:   asks,
	}
}

func (fixture symbolBookFixture) delta(
	bidPrice, bidQty, askPrice, askQty float64,
) krakenmarket.BookUpdate {
	update := fixture.snapshot(bidPrice, bidQty, askPrice, askQty)
	update.Type = ""

	return update
}

func advanceFluidGrid(
	state *FluidSymbol,
	fixture symbolBookFixture,
	at time.Time,
	bidPrice, bidQty, askPrice, askQty float64,
) error {
	if err := state.FeedBook(fixture.snapshot(bidPrice, bidQty, askPrice, askQty), at); err != nil {
		return err
	}

	return state.FeedBook(
		fixture.snapshot(bidPrice, bidQty, askPrice, askQty),
		at.Add(100*time.Millisecond),
	)
}

func TestFluidSymbolBuffersTradeBeforeBookSnapshot(t *testing.T) {
	Convey("Given a trade before the first book snapshot", t, func() {
		symbol := "ETH/EUR"
		viper.Set("market.book_depth_levels", 10)
		viper.Set("signals.volume_clock_bars_per_day", 288)
		viper.Set("signals.fluid.tick_size", 0.01)
		viper.Set("signals.fluid.grid_half_width", 10)
		viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
		feedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		state, err := NewFluidSymbol(symbol)
		So(err, ShouldBeNil)
		fixture := symbolBookFixture{symbol: symbol}
		tradeAt := feedAt.Add(10 * time.Millisecond)

		So(state.FeedTrade(tradeAt, 100, 1.5, "buy"), ShouldBeNil)

		Convey("It should buffer the trade until mid price exists", func() {
			So(len(state.bufferedTrades), ShouldEqual, 1)
			So(state.grid.lastMidPrice, ShouldEqual, 0)
		})

		Convey("When the first snapshot arrives", func() {
			So(state.FeedBook(fixture.snapshot(99, 10, 101, 6), feedAt), ShouldBeNil)

			Convey("It should flush the buffered trade into the grid", func() {
				So(len(state.bufferedTrades), ShouldEqual, 0)
				So(state.grid.lastMidPrice, ShouldBeGreaterThan, 0)

				index := state.grid.priceIndex(state.grid.lastMidPrice, 100)
				So(index, ShouldBeGreaterThanOrEqualTo, 0)
				So(state.grid.tradeExecuteAccumulator[index], ShouldEqual, 1.5)
			})
		})
	})
}

func TestFluidSymbolSingleLevelSnapshotUsesFallbackTick(t *testing.T) {
	Convey("Given a single-level book snapshot", t, func() {
		symbol := "SHIB/EUR"
		viper.Set("market.book_depth_levels", 10)
		viper.Set("signals.volume_clock_bars_per_day", 288)
		viper.Set("signals.fluid.tick_size", 0.00000001)
		viper.Set("signals.fluid.grid_half_width", 10)
		viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
		feedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		state, err := NewFluidSymbol(symbol)
		So(err, ShouldBeNil)

		So(state.FeedBook(krakenmarket.BookUpdate{
			Symbol: symbol,
			Type:   "snapshot",
			Bids:   []krakenmarket.BookLevel{{Price: 0.00001, Qty: 1}},
			Asks:   []krakenmarket.BookLevel{{Price: 0.00002, Qty: 1}},
		}, feedAt), ShouldBeNil)

		Convey("It should configure the grid from the fallback tick size", func() {
			So(state.grid.tickSize, ShouldEqual, 0.00000001)
		})
	})
}

func TestFluidSymbolConfigureTick(t *testing.T) {
	Convey("Given a symbol before its first book snapshot", t, func() {
		viper.Set("market.book_depth_levels", 10)
		viper.Set("signals.fluid.tick_size", 0.01)
		viper.Set("signals.fluid.grid_half_width", 10)
		viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
		state, err := NewFluidSymbol("BTC/USD")
		So(err, ShouldBeNil)

		Convey("When the exchange price increment arrives", func() {
			So(state.ConfigureTick(0.1), ShouldBeNil)
			So(state.grid.tickSize, ShouldEqual, 0.1)
		})
	})
}

func TestFluidSymbolBuffersTradeOutsideGridSpan(t *testing.T) {
	Convey("Given a trade outside the current grid span", t, func() {
		symbol := "BTC/USD"
		viper.Set("market.book_depth_levels", 10)
		viper.Set("signals.volume_clock_bars_per_day", 288)
		viper.Set("signals.fluid.tick_size", 0.01)
		viper.Set("signals.fluid.grid_half_width", 10)
		viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
		feedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		state, err := NewFluidSymbol(symbol)
		So(err, ShouldBeNil)
		fixture := symbolBookFixture{symbol: symbol}

		So(state.FeedBook(fixture.snapshot(99, 10, 101, 6), feedAt), ShouldBeNil)
		So(state.FeedTrade(feedAt.Add(10*time.Millisecond), 105, 1.5, "buy"), ShouldBeNil)

		Convey("It should buffer the trade instead of erroring", func() {
			So(len(state.bufferedTrades), ShouldEqual, 1)

			total := 0.0

			for _, qty := range state.grid.tradeExecuteAccumulator {
				total += qty
			}

			So(total, ShouldEqual, 0)
		})

		Convey("When a later book update recenters mid near the trade", func() {
			So(state.FeedBook(fixture.snapshot(104.5, 10, 105.5, 6), feedAt.Add(20*time.Millisecond)), ShouldBeNil)

			Convey("It should flush the buffered trade into the grid", func() {
				So(len(state.bufferedTrades), ShouldEqual, 0)

				index := state.grid.priceIndex(state.grid.lastMidPrice, 105)
				So(index, ShouldBeGreaterThanOrEqualTo, 0)
				So(state.grid.tradeExecuteAccumulator[index], ShouldEqual, 1.5)
			})
		})
	})
}

func TestFluidSymbolIgnoresFluxBeforeVolumeClock(t *testing.T) {
	Convey("Given book and trade updates before the volume clock is seeded", t, func() {
		symbol := "ETH/EUR"
		viper.Set("market.book_depth_levels", 10)
		viper.Set("signals.volume_clock_bars_per_day", 288)
		viper.Set("signals.fluid.tick_size", 0.01)
		viper.Set("signals.fluid.grid_half_width", 10)
		viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
		feedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		state, err := NewFluidSymbol(symbol)
		So(err, ShouldBeNil)
		fixture := symbolBookFixture{symbol: symbol}

		So(state.FeedBook(fixture.snapshot(99, 10, 101, 6), feedAt), ShouldBeNil)
		So(state.FeedTrade(feedAt, 100, 1, "buy"), ShouldBeNil)

		Convey("It should wait for ticker volume before accepting flux", func() {
			So(state.flux.hasTarget(), ShouldBeFalse)
		})
	})
}

func TestFluidSymbolRejectsDeltaBeforeSnapshot(t *testing.T) {
	Convey("Given a fluid symbol fed a delta before any snapshot", t, func() {
		symbol := "ETH/EUR"
		viper.Set("market.book_depth_levels", 10)
		viper.Set("signals.volume_clock_bars_per_day", 288)
		viper.Set("signals.fluid.tick_size", 0.01)
		viper.Set("signals.fluid.grid_half_width", 10)
		viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
		feedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		state, err := NewFluidSymbol(symbol)
		So(err, ShouldBeNil)
		fixture := symbolBookFixture{symbol: symbol}

		delta := fixture.delta(99, 10, 101, 6)
		So(state.FeedBook(delta, feedAt), ShouldBeNil)

		Convey("It should not treat the book as ready", func() {
			So(state.HasBook(), ShouldBeFalse)
		})

		Convey("It should report Reading as not ready", func() {
			_, ok := state.Reading()

			So(ok, ShouldBeFalse)
		})
	})
}

func TestFluidSymbolMeasureLaminarField(t *testing.T) {
	Convey("Given a balanced book with no Reynolds activity", t, func() {
		symbol := "BTC/EUR"
		viper.Set("market.book_depth_levels", 10)
		viper.Set("signals.volume_clock_bars_per_day", 288)
		viper.Set("signals.fluid.tick_size", 0.01)
		viper.Set("signals.fluid.grid_half_width", 10)
		viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
		feedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		state, err := NewFluidSymbol(symbol)
		So(err, ShouldBeNil)
		fixture := symbolBookFixture{symbol: symbol}

		So(state.FeedTicker(krakenmarket.TickerUpdate{
			Symbol: symbol, Last: 100, Bid: 99.99, Ask: 100.01, Volume: 1000,
		}, feedAt), ShouldBeNil)
		So(state.FeedBook(fixture.snapshot(99.99, 5, 100.01, 5), feedAt), ShouldBeNil)

		replenish := fixture.snapshot(99.99, 8, 100.01, 8)
		So(state.FeedBook(replenish, feedAt.Add(100*time.Millisecond)), ShouldBeNil)

		reading, ok := state.Reading()
		signal := NewSignal(symbol, logic.NewEntity(logic.EntityBook), nil)
		category, _, _, _, _ := signal.classify(reading)

		Convey("It should still publish a laminar reading", func() {
			So(ok, ShouldBeTrue)
			So(category, ShouldEqual, logic.CategoryInertial)
		})
	})
}

func TestSignalMeasureBookAfterRecord(t *testing.T) {
	Convey("Given a book update already fed in Record", t, func() {
		symbol := "ETH/EUR"
		viper.Set("market.book_depth_levels", 10)
		viper.Set("signals.volume_clock_bars_per_day", 288)
		viper.Set("signals.fluid.tick_size", 0.01)
		viper.Set("signals.fluid.grid_half_width", 10)
		viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
		viper.Set("signals.fluid.measurements_capacity", 4)

		feedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		system := &System{}
		signal := NewSignal(symbol, logic.NewEntity(logic.EntityBook), system)
		signal.warmupRemaining = 0

		state, stateErr := NewFluidSymbol(symbol)

		So(stateErr, ShouldBeNil)

		system.symbols.Store(symbol, state)

		fixture := symbolBookFixture{symbol: symbol}
		book := fixture.snapshot(99, 10, 101, 6)
		book.Timestamp = feedAt

		signal.Record(&book)

		_, measureErr := signal.Measure(nil, feedAt)

		Convey("It should measure without re-feeding the same book event", func() {
			So(measureErr, ShouldBeNil)
		})
	})
}

func TestSignalMeasureDefersWithoutEntitySamples(t *testing.T) {
	Convey("Given a symbol state warmed by another entity", t, func() {
		symbol := "ETH/EUR"
		viper.Set("market.book_depth_levels", 10)
		viper.Set("signals.volume_clock_bars_per_day", 288)
		viper.Set("signals.fluid.tick_size", 0.01)
		viper.Set("signals.fluid.grid_half_width", 10)
		viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
		viper.Set("signals.fluid.measurements_capacity", 4)

		feedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		system := &System{}
		signal := NewSignal(symbol, logic.NewEntity(logic.EntityTick), system)
		state, stateErr := NewFluidSymbol(symbol)

		So(stateErr, ShouldBeNil)

		fixture := symbolBookFixture{symbol: symbol}
		So(state.FeedTicker(krakenmarket.TickerUpdate{
			Symbol: symbol,
			Last:   100,
			Bid:    99,
			Ask:    101,
			Volume: 1000,
		}, feedAt), ShouldBeNil)
		So(state.FeedBook(fixture.snapshot(99, 10, 101, 6), feedAt), ShouldBeNil)

		system.symbols.Store(symbol, state)

		measurement, measureErr := signal.Measure(nil, feedAt)

		Convey("It should wait for a timestamped sample on that entity ring", func() {
			So(measureErr, ShouldBeNil)
			So(measurement.Publishable(), ShouldBeFalse)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	symbol := "ETH/EUR"
	viper.Set("market.book_depth_levels", 10)
	viper.Set("signals.volume_clock_bars_per_day", 288)
	viper.Set("signals.fluid.tick_size", 0.01)
	viper.Set("signals.fluid.grid_half_width", 10)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
	feedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	state, err := NewFluidSymbol(symbol)

	if err != nil {
		b.Fatal(err)
	}

	fixture := symbolBookFixture{symbol: symbol}

	if err := state.FeedTicker(krakenmarket.TickerUpdate{
		Symbol: symbol, Last: 100, Bid: 99, Ask: 101, Volume: 1000,
	}, feedAt); err != nil {
		b.Fatal(err)
	}

	if err := advanceFluidGrid(state, fixture, feedAt, 99, 10, 101, 6); err != nil {
		b.Fatal(err)
	}

	signal := NewSignal(symbol, logic.NewEntity(logic.EntityBook), nil)

	b.ReportAllocs()

	for b.Loop() {
		reading, ok := state.Reading()

		if ok {
			_, _ = signal.publish(reading, feedAt)
		}
	}
}
