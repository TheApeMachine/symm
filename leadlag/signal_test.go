package leadlag

import (
	"context"
	"testing"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

func TestLeadLagMeasure(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewLeadLag(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	anchor := newSymbolState(asset.Pair{Wsname: anchorSymbol})
	anchor.changePct = 0.08
	signal.symbols.Store(anchorSymbol, anchor)

	lagger := newSymbolState(asset.Pair{Wsname: "ALT/EUR"})
	lagger.changePct = 0.02
	signal.symbols.Store("ALT/EUR", lagger)

	found := false

	for measurement := range signal.Measure() {
		found = true

		if measurement.Source != leadlagSource || measurement.Confidence <= 0 ||
			measurement.Confidence > 1 {
			t.Fatalf("unexpected measurement: %+v", measurement)
		}
	}

	if !found {
		t.Fatal("expected lag measurement")
	}
}

func TestLeadLagFeedbackScalesSymbol(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewLeadLag(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	anchor := newSymbolState(asset.Pair{Wsname: anchorSymbol})
	anchor.changePct = 0.08
	lagger := newSymbolState(asset.Pair{Wsname: "ALT/EUR"})
	lagger.changePct = 0.02
	signal.symbols.Store("ALT/EUR", lagger)

	before, ok := lagMeasurement(anchor, lagger, 0.75, 0.75)

	if !ok {
		t.Fatal("expected leadlag measurement before feedback")
	}

	signal.Feedback(engine.PredictionFeedback{
		Source:          leadlagSource,
		Symbol:          "ALT/EUR",
		PredictedReturn: 0.02,
		ActualReturn:    -0.02,
	})

	after, ok := lagMeasurement(anchor, lagger, 0.75, 0.75)

	if !ok {
		t.Fatal("expected leadlag measurement after feedback")
	}

	if after.Confidence >= before.Confidence {
		t.Fatalf("expected feedback to lower confidence, before=%v after=%v", before.Confidence, after.Confidence)
	}
}

func BenchmarkLeadLagMeasure(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	b.Cleanup(func() { pool.Close() })

	signal := NewLeadLag(ctx, pool)

	anchor := newSymbolState(asset.Pair{Wsname: anchorSymbol})
	anchor.changePct = 0.08
	signal.symbols.Store(anchorSymbol, anchor)

	alt := newSymbolState(asset.Pair{Wsname: "ALT/EUR"})
	alt.changePct = 0.02
	signal.symbols.Store("ALT/EUR", alt)

	eth := newSymbolState(asset.Pair{Wsname: "ETH/EUR"})
	eth.changePct = 0.03
	signal.symbols.Store("ETH/EUR", eth)

	b.ReportAllocs()

	for b.Loop() {
		for range signal.Measure() {
		}
	}
}
