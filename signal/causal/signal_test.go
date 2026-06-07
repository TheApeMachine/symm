package causal

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/bus"
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
		pool := qpool.NewQ[any](ctx, 2, 4, qpool.NewConfig())
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
		pool := qpool.NewQ[any](ctx, 2, 4, qpool.NewConfig())
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

			waitCtx, waitCancel := context.WithTimeout(ctx, time.Second)
			defer waitCancel()

			value, err := bus.PollFor(waitCtx, measurements)
			if err != nil {
				t.Fatal("timed out waiting for causal measurement")
			}

			measurement, _ := value.Value.(types.Measurement)

			Convey("It publishes a structural reading", func() {
				So(measurement.Source, ShouldEqual, types.SourceCausal)
				So(measurement.Symbol, ShouldNotBeEmpty)
				So(measurement.Category, ShouldNotBeEmpty)
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

func TestMacroMomentumInsufficientPeers(t *testing.T) {
	Convey("Given fewer than two peer changes", t, func() {
		signal := &Signal{}
		signal.state("BTC/EUR").FeedTicker(market.TickerUpdate{ChangePct: 2.0})
		signal.state("ETH/EUR").FeedTicker(market.TickerUpdate{ChangePct: 1.0})

		Convey("It should return zero macro momentum", func() {
			So(signal.macroMomentum("BTC/EUR"), ShouldEqual, 0)
		})
	})
}

func TestThrottle(t *testing.T) {
	Convey("Given a recently published signal", t, func() {
		signal := &Signal{lastPublish: time.Now()}

		Convey("It should reject an immediate refit on a calm market", func() {
			So(signal.throttle(time.Now(), 0), ShouldBeFalse)
		})
	})

	Convey("Given a signal past the publish interval", t, func() {
		signal := &Signal{
			lastPublish: time.Now().Add(-causalPublishInterval - time.Millisecond),
		}

		Convey("It should allow another fit", func() {
			So(signal.throttle(time.Now(), 0), ShouldBeTrue)
		})
	})
}

func TestContagion(t *testing.T) {
	Convey("Given symbols with co-moving HY histories", t, func() {
		signal := &Signal{}
		base := int64(time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC).UnixNano())
		minSamples := contagionMinSamples()

		for index, symbol := range []string{"BTC/EUR", "ETH/EUR", "SOL/EUR"} {
			state := signal.state(symbol)
			offset := float64(index) * 10

			for sampleIndex := range minSamples + 2 {
				state.FeedTrade(market.TradeUpdate{
					Symbol:    symbol,
					Price:     100 + offset + float64(sampleIndex)*0.5,
					Qty:       1,
					Timestamp: time.Unix(0, base+int64(sampleIndex)*int64(time.Millisecond)),
				})
			}
		}

		Convey("It should report elevated cross-asset coupling", func() {
			So(signal.contagion(), ShouldBeGreaterThan, 0.5)
		})
	})
}

func BenchmarkMacroMomentum(b *testing.B) {
	signal := &Signal{}

	for index, symbol := range []string{"BTC/EUR", "ETH/EUR", "SOL/EUR", "ADA/EUR"} {
		signal.state(symbol).FeedTicker(market.TickerUpdate{
			ChangePct: 1 + float64(index)*0.3,
		})
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = signal.macroMomentum("ETH/EUR")
	}
}

func BenchmarkContagion(b *testing.B) {
	signal := &Signal{}
	base := int64(time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC).UnixNano())
	minSamples := contagionMinSamples()

	for index, symbol := range []string{"BTC/EUR", "ETH/EUR", "SOL/EUR", "ADA/EUR"} {
		state := signal.state(symbol)
		offset := float64(index) * 10

		for sampleIndex := range minSamples + 2 {
			state.FeedTrade(market.TradeUpdate{
				Symbol:    symbol,
				Price:     100 + offset + float64(sampleIndex)*0.5,
				Qty:       1,
				Timestamp: time.Unix(0, base+int64(sampleIndex)*int64(time.Millisecond)),
			})
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = signal.contagion()
	}
}

func TestPublishThrottled(t *testing.T) {
	Convey("Given a signal that just published", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		measurements := signal.broadcasts["measurements"].Subscribe("test:throttle", 64)
		signal.state("BTC/EUR").FeedTicker(market.TickerUpdate{Last: 100, ChangePct: 1.0})
		signal.state("ETH/EUR").FeedTicker(market.TickerUpdate{ChangePct: 0.5})
		signal.state("SOL/EUR").FeedTicker(market.TickerUpdate{ChangePct: 1.5})

		signal.lastPublish = time.Time{}
		signal.publish()

		waitCtx, waitCancel := context.WithTimeout(ctx, time.Second)
		defer waitCancel()

		if _, err := bus.PollFor(waitCtx, measurements); err != nil {
			t.Fatal("timed out waiting for first publish")
		}

		signal.publish()

		Convey("It should not emit a second reading inside the interval", func() {
			throttleCtx, throttleCancel := context.WithTimeout(ctx, 50*time.Millisecond)
			defer throttleCancel()

			if value, err := bus.PollFor(throttleCtx, measurements); err == nil {
				t.Fatalf("unexpected throttled measurement: %+v", value.Value)
			}
		})
	})
}

func BenchmarkPublish(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ[any](ctx, 2, 4, qpool.NewConfig())
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
