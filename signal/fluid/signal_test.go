package fluid

import (
	"context"
	"io"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/qpool"
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

func seedFluidConfig() {
	viper.Set("market.book_depth_levels", 10)
	viper.Set("signals.volume_clock_bars_per_day", 288)
	viper.Set("signals.fluid.tick_size", 0.01)
	viper.Set("signals.fluid.grid_half_width", 10)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
	symbolConfigValue.Store(nil)
}

func measurementArtifact(scope string) *datura.Artifact {
	artifact := datura.Acquire("measurement", datura.Artifact_Type_json)
	artifact.WithRole("measurement")
	artifact.WithScope(scope)

	return artifact
}

func newTestSignal(testingTB *testing.T) *Signal {
	testingTB.Helper()

	pool := qpool.NewQ[any](testingTB.Context(), 1, 2, nil)
	signal := NewSignal(testingTB.Context(), pool)
	testingTB.Cleanup(func() {
		_ = signal.Close()
	})

	return signal
}

func TestFluidSymbolMeasureLaminarField(testingTB *testing.T) {
	Convey("Given a balanced book with no Reynolds activity", testingTB, func() {
		seedFluidConfig()
		symbol := "BTC/EUR"
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

		registry := NewSyncRegistry()
		registry.symbols.Store(symbol, state)
		features := NewFeatures(context.Background(), registry)
		stage := algorithm.NewFluidflow()

		features.scope = symbol
		frame := make([]byte, 8192)
		readCount, readErr := features.Read(frame)

		if readErr == io.EOF && readCount > 0 {
			readErr = nil
		}

		So(readErr, ShouldBeNil)
		So(readCount, ShouldBeGreaterThan, 0)

		_, writeErr := stage.Write(frame[:readCount])
		So(writeErr, ShouldBeNil)

		_, readStageErr := stage.Read(frame)

		if readStageErr == io.EOF {
			readStageErr = nil
		}

		So(readStageErr, ShouldBeNil)

		outcome := stage.Outcome()

		Convey("It should publish an eligible flow reading", func() {
			So(outcome.Eligible, ShouldBeTrue)
			So(outcome.Strength, ShouldBeGreaterThan, 0)
			So(fluidCategory(outcome.Category), ShouldNotEqual, logic.CategoryTypeNone)
		})
	})
}

func TestSignalMeasureBookAfterFeed(testingTB *testing.T) {
	Convey("Given a warmed fluid symbol in the registry", testingTB, func() {
		seedFluidConfig()
		symbol := "ETH/EUR"
		feedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		signal := newTestSignal(testingTB)
		fixture := symbolBookFixture{symbol: symbol}
		state := signal.registry.loadSymbol(symbol)

		So(state.FeedTicker(krakenmarket.TickerUpdate{
			Symbol: symbol, Last: 100, Bid: 99, Ask: 101, Volume: 1000,
		}, feedAt), ShouldBeNil)
		So(state.FeedBook(fixture.snapshot(99, 10, 101, 6), feedAt), ShouldBeNil)
		So(state.FeedBook(
			fixture.snapshot(99, 10, 101, 6),
			feedAt.Add(100*time.Millisecond),
		), ShouldBeNil)

		_, measureErr := signal.Measure(measurementArtifact(symbol))

		Convey("It should measure without error once the grid is ready", func() {
			So(measureErr, ShouldBeNil)
		})
	})
}

func BenchmarkSignalMeasure(testingTB *testing.B) {
	seedFluidConfig()
	symbol := "ETH/EUR"
	feedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	signal := NewSignal(context.Background(), qpool.NewQ[any](context.Background(), 2, 4, nil))
	fixture := symbolBookFixture{symbol: symbol}

	signal.ticker.Update(krakenmarket.TickerUpdates{{
		Symbol: symbol, Last: 100, Bid: 99, Ask: 101, Volume: 1000, Timestamp: feedAt,
	}})

	book := fixture.snapshot(99, 10, 101, 6)
	book.Timestamp = feedAt
	signal.book.Update(krakenmarket.BookUpdates{&book})

	advanceBook := fixture.snapshot(99, 10, 101, 6)
	advanceBook.Timestamp = feedAt.Add(100 * time.Millisecond)
	signal.book.Update(krakenmarket.BookUpdates{&advanceBook})

	artifact := measurementArtifact(symbol)

	testingTB.ReportAllocs()

	for testingTB.Loop() {
		_, _ = signal.Measure(artifact)
	}
}
