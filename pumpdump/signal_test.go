package pumpdump

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

func testPumpDump(t *testing.T) (*PumpDump, *PumpSymbol) {
	t.Helper()

	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewPumpDump(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	signal.symbols["PUMP/EUR"] = NewPumpSymbol(asset.Pair{Wsname: "PUMP/EUR"})

	symbolState := signal.symbols["PUMP/EUR"]

	if symbolState == nil {
		t.Fatal("expected pump symbol state")
	}

	return signal, symbolState
}

func seedPumpSymbol(symbolState *PumpSymbol) {
	symbolState.lastPrice = 1
	symbolState.dailyQuoteVol = 50
	symbolState.imbalance = 0.8
	symbolState.buyPressure = 0.6
	symbolState.spreadBPS = 10

	for range 12 {
		_, _ = symbolState.volumeBaseline.Next(0, 10)
	}

	for range 8 {
		_, _ = symbolState.score.Push(1, 0.8, 0.6, 20, 1, 1, 1)
	}

	now := time.Unix(1_700_000_000, 0)
	_, _ = symbolState.volumeWindow.Next(0, float64(now.UnixNano()), 100, 1)
}

func TestPumpDumpMeasure(t *testing.T) {
	signal, symbolState := testPumpDump(t)
	seedPumpSymbol(symbolState)

	now := time.Unix(1_700_000_000, 0)
	found := false

	for measurement := range signal.Measure(context.Background(), now) {
		found = true

		if measurement.Source != pumpdumpSource || measurement.Confidence <= 0 {
			t.Fatalf("unexpected measurement: %+v", measurement)
		}
	}

	if !found {
		t.Fatal("expected at least one measurement")
	}
}

func TestPumpDumpFeedbackLowersConfidence(t *testing.T) {
	signal, symbolState := testPumpDump(t)
	seedPumpSymbol(symbolState)

	before := symbolState.forecast.Scale()

	signal.Feedback(engine.PredictionFeedback{
		Source:          pumpdumpSource,
		Symbol:          "PUMP/EUR",
		PredictedReturn: 0.01,
		ActualReturn:    -0.01,
	})

	if symbolState.forecast.Scale() >= before {
		t.Fatalf("expected scale to drop after loss, before=%v after=%v", before, symbolState.forecast.Scale())
	}

	now := time.Unix(1_700_000_000, 0)

	for measurement := range signal.Measure(context.Background(), now) {
		if measurement.Confidence <= 0 {
			t.Fatalf("expected positive confidence after feedback scale, got %+v", measurement)
		}
	}
}

func BenchmarkPumpDumpMeasure(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	signal := NewPumpDump(ctx, pool)
	signal.symbols["PUMP/EUR"] = NewPumpSymbol(asset.Pair{Wsname: "PUMP/EUR"})

	seedPumpSymbol(signal.symbols["PUMP/EUR"])

	now := time.Unix(1_700_000_000, 0)

	b.ReportAllocs()

	for b.Loop() {
		for range signal.Measure(ctx, now) {
		}
	}
}
