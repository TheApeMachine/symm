package depthflow

import (
	"context"
	"testing"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
)

func testDepthFlow(t *testing.T) (*DepthFlow, *DepthSymbol) {
	t.Helper()

	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewDepthFlow(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	signal.symbols["BTC/EUR"] = NewDepthSymbol(asset.Pair{Wsname: "BTC/EUR"})

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

	seedDepthSymbol(signal.symbols["BTC/EUR"])

	b.ReportAllocs()

	for b.Loop() {
		for range signal.Measure() {
		}
	}
}
