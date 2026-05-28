package leadlag

import (
	"context"
	"math"
	"testing"
	"time"

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

	returns := []float64{
		0.010, -0.007, 0.009, -0.006, 0.008, -0.005,
		0.007, -0.004, 0.006, -0.003, 0.005, -0.002,
		0.004, -0.003, 0.006, -0.004, 0.008, -0.005,
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	interval := 10 * time.Second

	anchor := newSymbolState(asset.Pair{Wsname: anchorSymbol})
	seedLeadLagWindow(anchor, 100, returns, start, interval, 0, 0.08)
	signal.symbols.Store(anchorSymbol, anchor)

	lagger := newSymbolState(asset.Pair{Wsname: "ALT/EUR"})
	seedLeadLagWindow(lagger, 50, returns, start, interval, interval, 0.02)
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
		Source:          engine.PerspectiveSource(engine.PerspectiveCrossAsset),
		Sources:         []string{leadlagSource},
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

	returns := []float64{
		0.010, -0.007, 0.009, -0.006, 0.008, -0.005,
		0.007, -0.004, 0.006, -0.003, 0.005, -0.002,
		0.004, -0.003, 0.006, -0.004, 0.008, -0.005,
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	interval := 10 * time.Second

	anchor := newSymbolState(asset.Pair{Wsname: anchorSymbol})
	seedLeadLagWindow(anchor, 100, returns, start, interval, 0, 0.08)
	signal.symbols.Store(anchorSymbol, anchor)

	alt := newSymbolState(asset.Pair{Wsname: "ALT/EUR"})
	seedLeadLagWindow(alt, 50, returns, start, interval, interval, 0.02)
	signal.symbols.Store("ALT/EUR", alt)

	eth := newSymbolState(asset.Pair{Wsname: "ETH/EUR"})
	seedLeadLagWindow(eth, 80, returns, start, interval, 2*interval, 0.03)
	signal.symbols.Store("ETH/EUR", eth)

	b.ReportAllocs()

	for b.Loop() {
		for range signal.Measure() {
		}
	}
}

func seedLeadLagWindow(
	state *symbolState,
	basePrice float64,
	returns []float64,
	start time.Time,
	interval time.Duration,
	offset time.Duration,
	changePct float64,
) {
	price := basePrice
	state.observeTicker(changePct, price, price*0.999, price*1.001, start.Add(offset))

	for index, logReturn := range returns {
		price *= math.Exp(logReturn)
		state.observeTicker(
			changePct,
			price,
			price*0.999,
			price*1.001,
			start.Add(time.Duration(index+1)*interval+offset),
		)
	}
}
