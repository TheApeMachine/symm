package sentiment

import (
	"context"
	"testing"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

func TestSentimentMeasure(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewSentiment(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	for index, symbol := range []string{"A/EUR", "B/EUR", "C/EUR", "D/EUR", "E/EUR"} {
		state := newSymbolState(asset.Pair{Wsname: symbol})
		state.changePct = 0.005 + float64(index)*0.002
		signal.symbols.Store(symbol, state)
	}

	found := false

	for measurement := range signal.Measure() {
		found = true

		if measurement.Source != sentimentSource || measurement.Confidence <= 0 ||
			measurement.Confidence > 1 {
			t.Fatalf("unexpected measurement: %+v", measurement)
		}
	}

	if !found {
		t.Fatal("expected sentiment measurement")
	}
}

func TestSentimentFeedbackScalesSymbol(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewSentiment(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	state := newSymbolState(asset.Pair{Wsname: "A/EUR"})
	state.changePct = 0.01
	signal.symbols.Store("A/EUR", state)

	before := state.calibratedConfidence(
		signal.sentimentConfidence(0.8, state.changePct, 0.02, 0.01),
	)

	signal.Feedback(engine.PredictionFeedback{
		Source:          sentimentSource,
		Symbol:          "A/EUR",
		PredictedReturn: 0.02,
		ActualReturn:    -0.02,
	})

	after := state.calibratedConfidence(
		signal.sentimentConfidence(0.8, state.changePct, 0.02, 0.01),
	)

	if after >= before {
		t.Fatalf("expected feedback to lower confidence, before=%v after=%v", before, after)
	}
}

func BenchmarkSentimentMeasure(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	b.Cleanup(func() { pool.Close() })

	signal := NewSentiment(ctx, pool)

	for index, symbol := range []string{"A/EUR", "B/EUR", "C/EUR", "D/EUR", "E/EUR"} {
		state := newSymbolState(asset.Pair{Wsname: symbol})
		state.changePct = 0.5 + float64(index)*0.2
		signal.symbols.Store(symbol, state)
	}

	b.ReportAllocs()

	for b.Loop() {
		for range signal.Measure() {
		}
	}
}
