package pumpdump

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
)

func storePumpSymbol(pumpdump *PumpDump, symbol string, state *PumpSymbol) {
	pumpdump.symbols.Store(symbol, state)
}

func loadPumpSymbol(pumpdump *PumpDump, symbol string) *PumpSymbol {
	raw, ok := pumpdump.symbols.Load(symbol)

	if !ok {
		return nil
	}

	return raw.(*PumpSymbol)
}

func markPumpRequested(pumpdump *PumpDump, symbol string) {
	pumpdump.requested.Store(symbol, struct{}{})
}

func testPumpDump(t *testing.T) (*PumpDump, *PumpSymbol, *qpool.Q) {
	t.Helper()

	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewPumpDump(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	storePumpSymbol(signal, "PUMP/EUR", NewPumpSymbol(asset.Pair{Wsname: "PUMP/EUR"}))
	markPumpRequested(signal, "PUMP/EUR")

	symbolState := loadPumpSymbol(signal, "PUMP/EUR")

	if symbolState == nil {
		t.Fatal("expected pump symbol state")
	}

	return signal, symbolState, pool
}

func seedPumpSymbol(symbolState *PumpSymbol) {
	symbolState.lastPrice = 1.003
	symbolState.dailyQuoteVol = 50
	symbolState.imbalance = 0.8
	symbolState.buyPressure = 0.6
	symbolState.spreadBPS = 10

	for range 12 {
		_, _ = symbolState.mediumVolumeBaseline.Next(0, 10)
	}

	for range 12 {
		_, _ = symbolState.fastVolumeBaseline.Next(0, 10)
	}

	for range 8 {
		_, _ = symbolState.score.Push(1, 0.8, 0.6, 20, 1, 1, 1)
	}

	_, _ = symbolState.spreadCompression.Next(15, 10)

	now := time.Unix(1_700_000_000, 0)
	_, _ = symbolState.mediumVolumeWindow.Next(0, float64(now.UnixNano()), 100, 1)
	_, _ = symbolState.fastVolumeWindow.Next(0, float64(now.UnixNano()), 100, 1)
}

func TestPumpdumpPublishPulseAfterBook(t *testing.T) {
	signal, state, pool := testPumpDump(t)
	seedPumpSymbol(state)

	measurements := signal.broadcasts["measurements"].Subscribe("test:pumpdump", 8)

	go func() {
		_ = signal.Tick()
	}()

	pool.CreateBroadcastGroup("book", 10*time.Millisecond).Send(&qpool.QValue[any]{
		Value: market.BookLevelsDelta{
			Symbol: "PUMP/EUR",
			Bids:   []market.BookLevel{{Price: 1, Volume: 80}},
			Asks:   []market.BookLevel{{Price: 1.01, Volume: 20}},
		},
	})

	select {
	case value := <-measurements.Incoming:
		measurement, ok := value.Value.(engine.Measurement)

		if !ok || measurement.Source != pumpdumpSource {
			t.Fatalf("expected pumpdump measurement, got %v", value.Value)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for pumpdump measurement after book tick")
	}
}

func TestPumpDumpMeasure(t *testing.T) {
	signal, symbolState, _ := testPumpDump(t)
	seedPumpSymbol(symbolState)

	found := false

	for measurement := range signal.Measure() {
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
	signal, symbolState, _ := testPumpDump(t)
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

	for measurement := range signal.Measure() {
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
	storePumpSymbol(signal, "PUMP/EUR", NewPumpSymbol(asset.Pair{Wsname: "PUMP/EUR"}))
	markPumpRequested(signal, "PUMP/EUR")

	seedPumpSymbol(loadPumpSymbol(signal, "PUMP/EUR"))

	b.ReportAllocs()

	for b.Loop() {
		for range signal.Measure() {
		}
	}
}
