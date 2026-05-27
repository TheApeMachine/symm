package fluid

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
)

func storeFluidSymbol(fluid *Fluid, symbol string, state *FluidSymbol) {
	fluid.symbols.Store(symbol, state)
}

func loadFluidSymbol(fluid *Fluid, symbol string) *FluidSymbol {
	raw, ok := fluid.symbols.Load(symbol)

	if !ok {
		return nil
	}

	return raw.(*FluidSymbol)
}

func markFluidRequested(fluid *Fluid, symbol string) {
	fluid.requested.Store(symbol, struct{}{})
}

func startFluidTick(t *testing.T, fluid *Fluid) {
	t.Helper()

	done := make(chan struct{})

	go func() {
		defer close(done)

		if err := fluid.Tick(); err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("fluid tick: %v", err)
		}
	}()

	t.Cleanup(func() {
		_ = fluid.Close()

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for fluid tick to close")
		}
	})
}

func TestFluidSymbolMeasure(t *testing.T) {
	state := NewFluidSymbol(asset.Pair{Wsname: "ALT/EUR"})
	state.bids = []market.BookLevel{{Price: 10, Volume: 80}}
	state.asks = []market.BookLevel{{Price: 10.01, Volume: 20}}
	state.spreadBPS = 10
	state.buyPressure, _ = state.pressure.Next(0, 0.8)

	for range 8 {
		_, _ = state.score.Push(0.7, 0.8)
	}

	for range 6 {
		measurement, ok := state.Measure()

		if ok && measurement.Confidence > 0 {
			return
		}
	}

	t.Fatal("expected fluid measurement from imbalanced book")
}

func TestFluidPublishPulseAfterBook(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewFluid(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	state := NewFluidSymbol(asset.Pair{Wsname: "ALT/EUR"})
	state.buyPressure, _ = state.pressure.Next(0, 0.8)
	state.bids = []market.BookLevel{{Price: 10, Volume: 80}}
	state.asks = []market.BookLevel{{Price: 10.01, Volume: 20}}

	for range 8 {
		_, _ = state.score.Push(0.7, 0.8)
	}

	storeFluidSymbol(signal, "ALT/EUR", state)

	measurements := signal.broadcasts["measurements"].Subscribe("test:fluid", 8)
	startFluidTick(t, signal)

	pool.CreateBroadcastGroup("book", 0).Send(&qpool.QValue[any]{
		Value: market.BookLevelsDelta{
			Symbol: "ALT/EUR",
			BidOK:  true,
			AskOK:  true,
			Bids:   []market.BookLevel{{Price: 10, Volume: 80}},
			Asks:   []market.BookLevel{{Price: 10.01, Volume: 20}},
		},
	})

	select {
	case value := <-measurements.Incoming:
		measurement, ok := value.Value.(engine.Measurement)

		if !ok || measurement.Source != fluidSource {
			t.Fatalf("expected fluid measurement, got %v", value.Value)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for fluid measurement after book tick")
	}
}

func TestFluidTickAppliesBook(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewFluid(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	storeFluidSymbol(signal, "ALT/EUR", NewFluidSymbol(asset.Pair{Wsname: "ALT/EUR"}))
	startFluidTick(t, signal)

	pool.CreateBroadcastGroup("book", 0).Send(&qpool.QValue[any]{
		Value: market.BookLevelsDelta{
			Symbol: "ALT/EUR",
			BidOK:  true,
			AskOK:  true,
			Bids:   []market.BookLevel{{Price: 10, Volume: 50}},
			Asks:   []market.BookLevel{{Price: 10.02, Volume: 40}},
		},
	})

	deadline := time.Now().Add(time.Second)

	for time.Now().Before(deadline) {
		state := loadFluidSymbol(signal, "ALT/EUR")

		if state != nil && len(state.bids) == 1 && state.spreadBPS > 0 {
			return
		}

		time.Sleep(time.Millisecond)
	}

	state := loadFluidSymbol(signal, "ALT/EUR")

	if state == nil {
		t.Fatal("expected book state, got nil")
	}

	t.Fatalf("expected book state, got bids=%d spread=%v",
		len(state.bids), state.spreadBPS)
}

func BenchmarkFluidMeasure(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	signal := NewFluid(ctx, pool)
	defer signal.Close()

	state := NewFluidSymbol(asset.Pair{Wsname: "ALT/EUR"})
	state.bids = []market.BookLevel{{Price: 10, Volume: 70}}
	state.asks = []market.BookLevel{{Price: 10.01, Volume: 30}}
	state.spreadBPS = 10
	storeFluidSymbol(signal, "ALT/EUR", state)
	markFluidRequested(signal, "ALT/EUR")

	b.ReportAllocs()

	for b.Loop() {
		for range signal.Measure() {
		}
	}
}
