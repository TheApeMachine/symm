package depthflow

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trade"
)

func testDepthFlow(t *testing.T) (*DepthFlow, *DepthSymbol) {
	t.Helper()

	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewDepthFlow(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	signal.symbols["BTC/EUR"] = NewDepthSymbol(asset.Pair{Wsname: "BTC/EUR"})
	signal.requested["BTC/EUR"] = struct{}{}

	state := signal.symbols["BTC/EUR"]

	if state == nil {
		t.Fatal("expected depth symbol state")
	}

	return signal, state
}

func seedDepthSymbol(state *DepthSymbol) {
	state.bids = []market.BookLevel{{Volume: 80}, {Volume: 20}}
	state.asks = []market.BookLevel{{Volume: 10}, {Volume: 10}}
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
	signal.symbols["BTC/EUR"] = state
	signal.requested["BTC/EUR"] = struct{}{}
	seedDepthSymbol(state)

	measurements := signal.broadcasts["measurements"].Subscribe("test:depthflow", 8)

	pool.CreateBroadcastGroup("trade", 0).Send(&qpool.QValue[any]{
		Value: trade.Data{
			Symbol:    "BTC/EUR",
			Side:      "buy",
			Qty:       0.1,
			Price:     50000,
			Timestamp: time.Now(),
		},
	})

	if err := signal.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}

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

	if err := signal.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
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

func BenchmarkDepthFlowMeasure(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	signal := NewDepthFlow(ctx, pool)
	signal.symbols["BTC/EUR"] = NewDepthSymbol(asset.Pair{Wsname: "BTC/EUR"})
	signal.requested["BTC/EUR"] = struct{}{}

	seedDepthSymbol(signal.symbols["BTC/EUR"])

	b.ReportAllocs()

	for b.Loop() {
		for range signal.Measure() {
		}
	}
}
