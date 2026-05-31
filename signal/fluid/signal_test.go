package fluid

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
)

func bookSnapshot(symbol string, bidPrice, bidQty, askPrice, askQty float64) market.BookUpdate {
	update := market.BookUpdate{
		Symbol: symbol,
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

func TestNewSignal(t *testing.T) {
	Convey("Given a qpool", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool, nil)
		defer signal.Close()

		Convey("It should wire measurements and ui broadcasts", func() {
			So(signal.broadcasts["measurements"], ShouldNotBeNil)
			So(signal.ui, ShouldNotBeNil)
		})
	})
}

func TestEmit(t *testing.T) {
	Convey("Given a fluid signal with a measurements subscriber", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool, nil)
		defer signal.Close()

		measurements := signal.broadcasts["measurements"].Subscribe("test:fluid", 64)
		symbol := "ETH/EUR"

		state := signal.state(symbol)
		state.FeedTicker(market.TickerUpdate{Symbol: symbol, Last: 100, Bid: 99, Ask: 101, Volume: 1000})
		state.FeedBook(bookSnapshot(symbol, 99, 10, 101, 6))

		Convey("When the field is measured after a book frame", func() {
			signal.emit(symbol)

			var measurement perspectives.Measurement
			received := false
			deadline := time.After(time.Second)

			for !received {
				select {
				case value := <-measurements.Incoming:
					reading, ok := value.Value.(perspectives.Measurement)

					if ok {
						measurement = reading
						received = true
					}
				case <-deadline:
					t.Fatal("timed out waiting for fluid measurement")
				}
			}

			Convey("It publishes a mechanical perspective reading", func() {
				So(measurement.Source, ShouldEqual, perspectives.SourceFluid)
				So(measurement.Symbol, ShouldEqual, symbol)
				So(measurement.SNR, ShouldBeGreaterThanOrEqualTo, 0)
			})
		})
	})
}

func TestPublishField(t *testing.T) {
	Convey("Given a fluid signal bound to the focus set", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		tracker := focus.NewSet()
		tracker.Add("ALGO/EUR")

		signal := NewSignal(ctx, pool, tracker)
		defer signal.Close()

		uiFrames := signal.ui.Subscribe("test:fluid-ui", 8)

		unfocused := signal.state("ETH/EUR")
		unfocused.FeedTicker(market.TickerUpdate{
			Symbol: "ETH/EUR", Last: 100, Bid: 99, Ask: 101, Volume: 1000,
		})
		unfocused.FeedBook(bookSnapshot("ETH/EUR", 99, 10, 101, 6))

		anchor := signal.state(focus.AnchorSymbol)
		anchor.FeedTicker(market.TickerUpdate{
			Symbol: focus.AnchorSymbol, Last: 100, Bid: 99, Ask: 101, Volume: 1000,
		})
		anchor.FeedBook(bookSnapshot(focus.AnchorSymbol, 99, 10, 101, 6))

		signal.publishField(focus.AnchorSymbol, anchor)

		select {
		case value := <-uiFrames.Incoming:
			frame, ok := value.Value.(map[string]any)

			So(ok, ShouldBeTrue)
			So(frame["event"], ShouldEqual, "field_snapshot")

			symbols, ok := frame["symbols"].([]map[string]any)

			So(ok, ShouldBeTrue)
			So(len(symbols), ShouldBeGreaterThanOrEqualTo, 2)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for field snapshot")
		}
	})
}

func BenchmarkEmit(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	signal := NewSignal(ctx, pool, nil)
	defer signal.Close()

	signal.broadcasts["measurements"].Subscribe("bench:fluid", 1024)

	symbol := "ETH/EUR"
	state := signal.state(symbol)
	state.FeedTicker(market.TickerUpdate{Symbol: symbol, Last: 100, Bid: 99, Ask: 101, Volume: 1000})
	state.FeedBook(bookSnapshot(symbol, 99, 10, 101, 6))

	b.ReportAllocs()

	for b.Loop() {
		signal.emit(symbol)
	}
}
