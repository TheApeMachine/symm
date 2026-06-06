package fluid

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func bookSnapshot(symbol string, bidPrice, bidQty, askPrice, askQty float64) market.Book {
	update := market.Book{
		Symbol: symbol,
		Bids:   []market.BookLevel{{Price: bidPrice, Qty: bidQty}},
		Asks:   []market.BookLevel{{Price: askPrice, Qty: askQty}},
	}
	update.SetEnvelopeType(market.BookSnapshot)

	return update
}

func TestNewSignal(t *testing.T) {
	Convey("Given a qpool", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		Convey("It should wire measurements and ui broadcasts", func() {
			So(signal.broadcasts["measurements"], ShouldNotBeNil)
			So(signal.ui, ShouldNotBeNil)
		})
	})
}

func TestEmit(t *testing.T) {
	Convey("Given a fluid signal with a measurements subscriber", t, func() {
		t.Cleanup(viper.Reset)
		viper.Set("market.book_depth_levels", 10)
		viper.Set("signals.volume_clock_bars_per_day", 288)

		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		measurements := signal.broadcasts["measurements"].Subscribe("test:fluid", 64)
		symbol := "ETH/EUR"

		state, err := signal.state(symbol)
		So(err, ShouldBeNil)
		So(state.FeedTicker(market.TickerUpdate{Symbol: symbol, Last: 100, Bid: 99, Ask: 101, Volume: 1000}), ShouldBeNil)
		So(state.FeedBook(bookSnapshot(symbol, 99, 10, 101, 6)), ShouldBeNil)

		Convey("When the field is measured after a book frame", func() {
			signal.emit(symbol)

			var measurement types.Measurement
			received := false
			deadline := time.After(time.Second)

			for !received {
				select {
				case value := <-measurements.Incoming:
					reading, ok := value.Value.(types.Measurement)

					if ok {
						measurement = reading
						received = true
					}
				case <-deadline:
					t.Fatal("timed out waiting for fluid measurement")
				}
			}

			Convey("It publishes a mechanical perspective reading", func() {
				So(measurement.Source, ShouldEqual, types.SourceFluid)
				So(measurement.Symbol, ShouldEqual, symbol)
				So(measurement.SNR, ShouldBeGreaterThanOrEqualTo, 0)
			})
		})
	})
}

func TestPublishField(t *testing.T) {
	Convey("Given a fluid signal with multiple symbols in the universe", t, func() {
		t.Cleanup(viper.Reset)
		viper.Set("market.anchor_symbol", "BTC/EUR")
		viper.Set("market.default_symbols", []string{"BTC/EUR"})
		viper.Set("market.book_depth_levels", 10)
		viper.Set("signals.volume_clock_bars_per_day", 288)
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		uiFrames := signal.ui.Subscribe("test:fluid-ui", 8)

		unfocused, err := signal.state("ETH/EUR")
		So(err, ShouldBeNil)
		So(unfocused.FeedTicker(market.TickerUpdate{
			Symbol: "ETH/EUR", Last: 100, Bid: 99, Ask: 101, Volume: 1000,
		}), ShouldBeNil)
		So(unfocused.FeedBook(bookSnapshot("ETH/EUR", 99, 10, 101, 6)), ShouldBeNil)

		anchor, err := signal.state(focus.AnchorSymbol())
		So(err, ShouldBeNil)
		So(anchor.FeedTicker(market.TickerUpdate{
			Symbol: focus.AnchorSymbol(), Last: 100, Bid: 99, Ask: 101, Volume: 1000,
		}), ShouldBeNil)
		So(anchor.FeedBook(bookSnapshot(focus.AnchorSymbol(), 99, 10, 101, 6)), ShouldBeNil)

		if err := signal.publishField(anchor); err != nil {
			t.Fatal(err)
		}

		select {
		case value := <-uiFrames.Incoming:
			frame, ok := value.Value.(map[string]any)

			So(ok, ShouldBeTrue)
			So(frame["type"], ShouldEqual, "fluid")

			symbols, ok := frame["symbols"].([]map[string]any)

			So(ok, ShouldBeTrue)
			So(len(symbols), ShouldBeGreaterThanOrEqualTo, 2)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for field snapshot")
		}
	})
}

func TestPublishFieldSkipsUnwarmedSymbols(t *testing.T) {
	Convey("Given symbols without a priced field yet", t, func() {
		t.Cleanup(viper.Reset)
		viper.Set("market.book_depth_levels", 10)
		viper.Set("signals.volume_clock_bars_per_day", 288)

		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		uiFrames := signal.ui.Subscribe("test:fluid-ui-empty", 1)

		unpriced, err := signal.state("ETH/EUR")
		So(err, ShouldBeNil)
		_, err = signal.state("SOL/EUR")
		So(err, ShouldBeNil)

		Convey("It should not treat missing rows as errors", func() {
			So(signal.publishField(unpriced), ShouldBeNil)

			select {
			case <-uiFrames.Incoming:
				t.Fatal("unexpected field snapshot for unwarmed symbols")
			case <-time.After(50 * time.Millisecond):
			}
		})
	})
}

func TestEmitSkipsMeasurementWithoutLast(t *testing.T) {
	Convey("Given ticker volume before a last price", t, func() {
		t.Cleanup(viper.Reset)
		viper.Set("market.book_depth_levels", 10)
		viper.Set("signals.volume_clock_bars_per_day", 288)

		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		measurements := signal.broadcasts["measurements"].Subscribe("test:fluid-no-last", 4)
		symbol := "ETH/EUR"

		state, err := signal.state(symbol)
		So(err, ShouldBeNil)
		So(state.FeedTicker(market.TickerUpdate{Symbol: symbol, Volume: 1000}), ShouldBeNil)

		So(signal.emit(symbol), ShouldBeNil)

		select {
		case <-measurements.Incoming:
			t.Fatal("unexpected measurement without last price")
		case <-time.After(50 * time.Millisecond):
		}
	})
}

func BenchmarkPublishField(b *testing.B) {
	viper.Set("market.book_depth_levels", 10)
	viper.Set("signals.volume_clock_bars_per_day", 288)

	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	signal := NewSignal(ctx, pool)
	defer pool.Close()

	for _, symbol := range []string{"ETH/EUR", "BTC/EUR", "SOL/EUR", "ADA/EUR"} {
		state, err := signal.state(symbol)

		if err != nil {
			b.Fatal(err)
		}

		if err := state.FeedTicker(market.TickerUpdate{
			Symbol: symbol, Last: 100, Bid: 99, Ask: 101, Volume: 1000,
		}); err != nil {
			b.Fatal(err)
		}

		if err := state.FeedBook(bookSnapshot(symbol, 99, 10, 101, 6)); err != nil {
			b.Fatal(err)
		}
	}

	trigger, err := signal.state("ETH/EUR")

	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		if err := signal.publishField(trigger); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEmit(b *testing.B) {
	viper.Set("market.book_depth_levels", 10)
	viper.Set("signals.volume_clock_bars_per_day", 288)

	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	signal := NewSignal(ctx, pool)
	defer signal.Close()

	signal.broadcasts["measurements"].Subscribe("bench:fluid", 1024)

	symbol := "ETH/EUR"
	state, err := signal.state(symbol)
	if err != nil {
		b.Fatal(err)
	}
	state.FeedTicker(market.TickerUpdate{Symbol: symbol, Last: 100, Bid: 99, Ask: 101, Volume: 1000})
	state.FeedBook(bookSnapshot(symbol, 99, 10, 101, 6))

	b.ReportAllocs()

	for b.Loop() {
		signal.emit(symbol)
	}
}
