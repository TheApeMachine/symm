package depthflow

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

		Convey("It should expose a measurements broadcast", func() {
			So(signal.broadcasts["measurements"], ShouldNotBeNil)
		})
	})
}

func TestObserveTrade(t *testing.T) {
	Convey("Given a depthflow signal with a measurements subscriber", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		measurements := signal.broadcasts["measurements"].Subscribe("test:depthflow", 64)
		symbol := "ETH/EUR"
		now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

		state := signal.state(symbol)
		state.ApplyBook(bookSnapshot(symbol, 99, 8, 101, 4))
		state.FeedTicker(market.TickerUpdate{Symbol: symbol, Last: 100, Bid: 99, Ask: 101})

		Convey("When buy pressure aligns with bid-heavy depth", func() {
			signal.observeTrade(market.TradeUpdate{
				Symbol: symbol, Side: "buy", Price: 100, Qty: 3, Timestamp: now,
			})

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
					t.Fatal("timed out waiting for depthflow measurement")
				}
			}

			Convey("It publishes a depthflow reading", func() {
				So(measurement.Source, ShouldEqual, perspectives.SourceDepthFlow)
				So(measurement.Symbol, ShouldEqual, symbol)
				So(measurement.SNR, ShouldBeGreaterThanOrEqualTo, 0)
			})
		})
	})
}

func BenchmarkObserveTrade(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	signal := NewSignal(ctx, pool)
	defer signal.Close()

	signal.broadcasts["measurements"].Subscribe("bench:depthflow", 1024)

	symbol := "ETH/EUR"
	state := signal.state(symbol)
	state.ApplyBook(bookSnapshot(symbol, 99, 8, 101, 4))
	state.FeedTicker(market.TickerUpdate{Symbol: symbol, Last: 100, Bid: 99, Ask: 101})

	trade := market.TradeUpdate{
		Symbol: symbol, Side: "buy", Price: 100, Qty: 3,
		Timestamp: time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC),
	}

	b.ReportAllocs()

	for b.Loop() {
		signal.observeTrade(trade)
	}
}
