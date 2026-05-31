package fluid

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
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
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
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

func BenchmarkEmit(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	signal := NewSignal(ctx, pool)
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
