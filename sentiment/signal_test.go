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
		signal.symbols[symbol] = &symbolState{
			pair:       asset.Pair{Wsname: symbol},
			changePct:  0.005 + float64(index)*0.002,
			confidence: engine.NewSymbolConfidence(engine.DefaultCalibrationParams()),
		}
		engine.WarmSymbolConfidence(signal.symbols[symbol].confidence, 0.002, 0.003, 0.004, 0.005)
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

func BenchmarkSentimentMeasure(b *testing.B) {
	signal := NewSentiment(context.Background(), nil)

	for index, symbol := range []string{"A/EUR", "B/EUR", "C/EUR", "D/EUR", "E/EUR"} {
		signal.symbols[symbol] = &symbolState{changePct: 0.5 + float64(index)*0.2}
	}

	b.ReportAllocs()

	for b.Loop() {
		for range signal.Measure() {
		}
	}
}
