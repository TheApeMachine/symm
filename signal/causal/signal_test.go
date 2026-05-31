package causal

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

func TestPublish(t *testing.T) {
	Convey("Given a causal signal with a measurements subscriber", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		measurements := signal.broadcasts["measurements"].Subscribe("test:causal", 64)
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

		symbols := []string{"BTC/EUR", "ETH/EUR", "SOL/EUR"}

		for index, symbol := range symbols {
			state := signal.state(symbol)
			state.FeedTicker(market.TickerUpdate{
				Symbol: symbol, Last: 100 + float64(index)*10,
				ChangePct: 1.0 + float64(index)*0.2,
				Volume:    1000, Bid: 99, Ask: 101,
			})
			state.FeedBook(bookSnapshot(symbol, 99, 8, 101, 6))

			for tradeIndex := range 12 {
				state.FeedTrade(market.TradeUpdate{
					Symbol: symbol, Side: "buy",
					Price:     100 + float64(index)*10,
					Qty:       2,
					Timestamp: base.Add(time.Duration(tradeIndex) * time.Millisecond),
				})
			}
		}

		signal.lastPublish = time.Time{}

		Convey("When the cross-section fit runs", func() {
			signal.publish()

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
					t.Fatal("timed out waiting for causal measurement")
				}
			}

			Convey("It publishes a structural reading", func() {
				So(measurement.Source, ShouldEqual, perspectives.SourceCausal)
				So(measurement.Symbol, ShouldNotBeEmpty)
				So(measurement.SNR, ShouldBeGreaterThanOrEqualTo, 0)
			})
		})
	})
}

func TestMacroMomentum(t *testing.T) {
	Convey("Given a causal signal with peer tickers", t, func() {
		signal := &Signal{}
		signal.state("BTC/EUR").FeedTicker(market.TickerUpdate{ChangePct: 2.0})
		signal.state("ETH/EUR").FeedTicker(market.TickerUpdate{ChangePct: 1.0})
		signal.state("SOL/EUR").FeedTicker(market.TickerUpdate{ChangePct: 3.0})

		Convey("It should exclude the candidate from the macro median", func() {
			macro := signal.macroMomentum("ETH/EUR")
			So(macro, ShouldAlmostEqual, 2.5, 0.01)
		})
	})
}

func BenchmarkPublish(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	signal := NewSignal(ctx, pool)
	defer signal.Close()

	signal.broadcasts["measurements"].Subscribe("bench:causal", 1024)

	for index, symbol := range []string{"BTC/EUR", "ETH/EUR", "SOL/EUR"} {
		state := signal.state(symbol)
		state.FeedTicker(market.TickerUpdate{
			Symbol: symbol, Last: 100 + float64(index)*10,
			ChangePct: 1.0, Volume: 1000, Bid: 99, Ask: 101,
		})
		state.FeedBook(bookSnapshot(symbol, 99, 8, 101, 6))
	}

	b.ReportAllocs()

	for b.Loop() {
		signal.lastPublish = time.Time{}
		signal.publish()
	}
}
