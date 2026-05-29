package fluid

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
)

func benchFluidSymbol() *FluidSymbol {
	state := NewFluidSymbol(asset.Pair{Wsname: "ALT/EUR"})
	state.bids = []market.BookLevel{{Price: 10, Volume: 80}}
	state.asks = []market.BookLevel{{Price: 10.01, Volume: 20}}
	state.spreadBPS = 10
	state.buyPressure, _ = state.pressure.Next(0, 0.8)

	for range 8 {
		_, _ = state.score.Push(0.7, 0.8)
	}

	return state
}

func benchBookDelta() market.BookLevelsDelta {
	return market.BookLevelsDelta{
		Symbol: "ALT/EUR",
		BidOK:  true,
		AskOK:  true,
		Bids:   []market.BookLevel{{Price: 10, Volume: 80}, {Price: 9.99, Volume: 40}},
		Asks:   []market.BookLevel{{Price: 10.01, Volume: 20}, {Price: 10.02, Volume: 30}},
	}
}

func BenchmarkFluidSymbolFeedBook(b *testing.B) {
	state := NewFluidSymbol(asset.Pair{Wsname: "ALT/EUR"})
	delta := benchBookDelta()

	// Prime previous snapshot so churn is measured, not skipped.
	state.FeedBook(delta)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		state.FeedBook(delta)
	}
}

func BenchmarkFluidSymbolFeedTicker(b *testing.B) {
	state := NewFluidSymbol(asset.Pair{Wsname: "ALT/EUR"})
	row := market.TickerRow{
		Symbol:    "ALT/EUR",
		Last:      10.5,
		Bid:       10.4,
		Ask:       10.6,
		Volume:    8640,
		ChangePct: 2.2,
	}

	b.ReportAllocs()

	for b.Loop() {
		state.FeedTicker(row)
	}
}

func BenchmarkFluidSymbolWireRow(b *testing.B) {
	state := benchFluidSymbol()
	state.changePct = 2.5
	state.volume = 1200

	b.ReportAllocs()

	for b.Loop() {
		_ = state.wireRow()
	}
}

func BenchmarkFluidSymbolMeasureBookFlow(b *testing.B) {
	state := benchFluidSymbol()
	now := time.Unix(1_700_000_000, 0)
	state.FeedTrade(now, 50)

	b.ReportAllocs()

	for b.Loop() {
		_, _ = state.Measure()
	}
}

func BenchmarkSideChangeFlux(b *testing.B) {
	previous := []market.BookLevel{
		{Price: 100, Volume: 10},
		{Price: 99.5, Volume: 5},
	}
	updated := []market.BookLevel{
		{Price: 100, Volume: 20},
		{Price: 99.5, Volume: 5},
		{Price: 99, Volume: 3},
	}

	b.ReportAllocs()

	for b.Loop() {
		sideChangeFlux(previous, updated)
	}
}

func BenchmarkFluxAddBookAddTrade(b *testing.B) {
	flux := newFluxAccumulator(time.Minute)
	flux.setTarget(10)
	now := time.Unix(1_700_000_000, 0)

	b.ReportAllocs()

	for b.Loop() {
		flux.addBook(now, 4)
		flux.addTrade(now, 1)
	}
}

func BenchmarkFluidPublishFieldRows(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	b.Cleanup(func() { pool.Close() })

	signal := NewFluid(ctx, pool)
	b.Cleanup(func() { _ = signal.Close() })

	state := benchFluidSymbol()
	state.changePct = 2.5
	state.volume = 1200
	storeFluidSymbol(signal, "ALT/EUR", state)
	markFluidRequested(signal, "ALT/EUR")

	b.ReportAllocs()

	for b.Loop() {
		signal.publishFieldRows()
	}
}

func BenchmarkFluidPublishPulse(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	b.Cleanup(func() { pool.Close() })

	signal := NewFluid(ctx, pool)
	b.Cleanup(func() { _ = signal.Close() })

	state := benchFluidSymbol()
	state.changePct = 2.5
	state.volume = 1200
	storeFluidSymbol(signal, "ALT/EUR", state)
	markFluidRequested(signal, "ALT/EUR")

	b.ReportAllocs()

	for b.Loop() {
		signal.publishPulse()
	}
}
