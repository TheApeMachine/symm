package depthflow

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trade"
)

func storeDepthSymbol(depthflow *DepthFlow, symbol string, state *DepthSymbol) {
	depthflow.symbols.Store(symbol, state)
}

func loadDepthSymbol(depthflow *DepthFlow, symbol string) *DepthSymbol {
	raw, ok := depthflow.symbols.Load(symbol)

	if !ok {
		return nil
	}

	return raw.(*DepthSymbol)
}

func markDepthRequested(depthflow *DepthFlow, symbol string) {
	depthflow.requested.Store(symbol, struct{}{})
}

func startDepthFlowTick(t *testing.T, depthflow *DepthFlow) {
	t.Helper()

	done := make(chan struct{})

	go func() {
		defer close(done)

		if err := depthflow.Tick(); err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("depthflow tick: %v", err)
		}
	}()

	t.Cleanup(func() {
		_ = depthflow.Close()

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for depthflow tick to close")
		}
	})
}

func testDepthFlow(t *testing.T) (*DepthFlow, *DepthSymbol) {
	t.Helper()

	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewDepthFlow(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	storeDepthSymbol(signal, "BTC/EUR", NewDepthSymbol(asset.Pair{Wsname: "BTC/EUR"}))
	markDepthRequested(signal, "BTC/EUR")

	state := loadDepthSymbol(signal, "BTC/EUR")

	if state == nil {
		t.Fatal("expected depth symbol state")
	}

	return signal, state
}

func seedDepthSymbol(state *DepthSymbol) {
	state.bids = []market.BookLevel{
		{Price: 100, Volume: 80},
		{Price: 99.5, Volume: 20},
	}
	state.asks = []market.BookLevel{
		{Price: 100.1, Volume: 10},
		{Price: 100.2, Volume: 10},
	}
	state.buyPressure = 0.6

	for range 8 {
		_, _ = state.score.Push(0.7, 0.8)
	}
}

func TestDepthFlowPublishPulseAfterTrade(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewDepthFlow(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	state := NewDepthSymbol(asset.Pair{Wsname: "BTC/EUR"})
	storeDepthSymbol(signal, "BTC/EUR", state)
	markDepthRequested(signal, "BTC/EUR")
	seedDepthSymbol(state)

	measurements := signal.broadcasts["measurements"].Subscribe("test:depthflow", 8)
	startDepthFlowTick(t, signal)

	pool.CreateBroadcastGroup("trade", 0).Send(&qpool.QValue[any]{
		Value: trade.Data{
			Symbol:    "BTC/EUR",
			Side:      "buy",
			Qty:       0.1,
			Price:     50000,
			Timestamp: time.Now(),
		},
	})

	select {
	case value := <-measurements.Incoming:
		measurement, ok := value.Value.(engine.Measurement)

		if !ok || measurement.Source != depthflowSource {
			t.Fatalf("expected depthflow measurement, got %v", value.Value)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for depthflow measurement after trade tick")
	}
}

func TestDepthFlowTickIgnoresUnknownTrade(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewDepthFlow(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	measurements := signal.broadcasts["measurements"].Subscribe("test:unknown-depthflow", 8)
	startDepthFlowTick(t, signal)

	trades := pool.CreateBroadcastGroup("trade", 10*time.Millisecond)
	trades.Send(&qpool.QValue[any]{
		Value: trade.Data{
			Symbol:    "BTC/EUR",
			Side:      "buy",
			Qty:       0.1,
			Price:     50000,
			Timestamp: time.Now(),
		},
	})

	select {
	case value := <-measurements.Incoming:
		t.Fatalf("expected no measurement for unknown trade, got %v", value.Value)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestDepthFlowMeasure(t *testing.T) {
	signal, state := testDepthFlow(t)
	seedDepthSymbol(state)

	found := false

	for measurement := range signal.Measure() {
		found = true

		if measurement.Source != depthflowSource || measurement.Confidence <= 0 {
			t.Fatalf("unexpected measurement: %+v", measurement)
		}

		if measurement.Type != engine.DepthFlow {
			t.Fatalf("expected depthflow type for bid-heavy book, got %+v", measurement.Type)
		}
	}

	if !found {
		t.Fatal("expected at least one measurement")
	}
}

func TestDepthFlowFeedbackLowersConfidence(t *testing.T) {
	signal, state := testDepthFlow(t)
	seedDepthSymbol(state)

	before := state.forecast.Scale()

	signal.Feedback(engine.PredictionFeedback{
		Source:          depthflowSource,
		Symbol:          "BTC/EUR",
		PredictedReturn: 0.01,
		ActualReturn:    -0.01,
	})

	if state.forecast.Scale() >= before {
		t.Fatalf("expected scale to drop after loss, before=%v after=%v", before, state.forecast.Scale())
	}

	for measurement := range signal.Measure() {
		if measurement.Confidence <= 0 {
			t.Fatalf("expected positive confidence after feedback scale, got %+v", measurement)
		}
	}
}

func TestDepthFlowConcurrentSymbolUpdates(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 4, 8, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewDepthFlow(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	for index := 0; index < 32; index++ {
		symbol := fmt.Sprintf("SYM%d/EUR", index)
		storeDepthSymbol(signal, symbol, NewDepthSymbol(asset.Pair{Wsname: symbol}))
		markDepthRequested(signal, symbol)
		seedDepthSymbol(loadDepthSymbol(signal, symbol))
	}

	var workers sync.WaitGroup

	for index := 0; index < 32; index++ {
		workers.Add(1)

		go func() {
			defer workers.Done()
			signal.publishMeasurements()
		}()
	}

	workers.Wait()
}

func TestDepthFlowQueuePendingDeduplicates(t *testing.T) {
	signal := &DepthFlow{}
	signal.queuePending("BTC/EUR")
	signal.queuePending("BTC/EUR")

	symbols := signal.pendingBatch(4, 0)

	if len(symbols) != 1 || symbols[0] != "BTC/EUR" {
		t.Fatalf("expected one pending symbol, got %v", symbols)
	}
}

func BenchmarkDepthFlowMeasure(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	signal := NewDepthFlow(ctx, pool)
	storeDepthSymbol(signal, "BTC/EUR", NewDepthSymbol(asset.Pair{Wsname: "BTC/EUR"}))
	markDepthRequested(signal, "BTC/EUR")

	seedDepthSymbol(loadDepthSymbol(signal, "BTC/EUR"))

	b.ReportAllocs()

	for b.Loop() {
		for range signal.Measure() {
		}
	}
}

func BenchmarkDepthFlowPublishMeasurements(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 4, 8, qpool.NewConfig())
	defer pool.Close()

	signal := NewDepthFlow(ctx, pool)

	for index := 0; index < 32; index++ {
		symbol := fmt.Sprintf("SYM%d/EUR", index)
		storeDepthSymbol(signal, symbol, NewDepthSymbol(asset.Pair{Wsname: symbol}))
		markDepthRequested(signal, symbol)
		seedDepthSymbol(loadDepthSymbol(signal, symbol))
	}

	b.ReportAllocs()

	for b.Loop() {
		signal.publishMeasurements()
	}
}
