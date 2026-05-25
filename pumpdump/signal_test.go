package pumpdump

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

func TestPumpDumpPublishesMeasurement(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	signal := NewPumpDump(ctx, pool)
	subscriber := signal.broadcasts["measurements"].Subscribe("test", 8)

	signal.symbols["PUMP/EUR"] = NewPumpSymbol(asset.Pair{Wsname: "PUMP/EUR"})

	sym := signal.symbols["PUMP/EUR"]
	sym.lastPrice = 1
	sym.dailyQuoteVol = 50
	sym.imbalance = 0.8
	sym.buyPressure = 0.6
	sym.spreadBPS = 10

	for range 12 {
		_, _ = sym.volumeBaseline.Next(0, 10)
	}

	for range 8 {
		_, _ = sym.score.Push(1, 0.8, 0.6, 20, 1, 1)
	}

	now := time.Unix(1_700_000_000, 0)
	_, _ = sym.volumeWindow.Next(0, float64(now.UnixNano()), 100, 1)

	if err := signal.scoreAll(); err != nil {
		t.Fatalf("score: %v", err)
	}

	select {
	case value := <-subscriber.Incoming:
		measurement, ok := value.Value.(engine.Measurement)

		if !ok {
			t.Fatalf("expected measurement, got %T", value.Value)
		}

		if measurement.Source != "pumpdump" || measurement.Confidence <= 0 {
			t.Fatalf("unexpected measurement: %+v", measurement)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for pumpdump measurement")
	}
}

func BenchmarkPumpDumpScoreAll(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	signal := NewPumpDump(ctx, pool)
	signal.symbols["PUMP/EUR"] = NewPumpSymbol(asset.Pair{Wsname: "PUMP/EUR"})

	sym := signal.symbols["PUMP/EUR"]
	sym.lastPrice = 1
	sym.dailyQuoteVol = 50
	sym.imbalance = 0.8
	sym.buyPressure = 0.6
	sym.spreadBPS = 10

	for range 6 {
		_, _ = sym.volumeBaseline.Next(0, 10)
	}

	for range 6 {
		_, _ = sym.score.Push(1, 0.8, 0.6, 20, 1, 1)
	}

	now := time.Unix(1_700_000_000, 0)
	_, _ = sym.volumeWindow.Next(0, float64(now.UnixNano()), 100, 1)

	b.ReportAllocs()

	for b.Loop() {
		if err := signal.scoreAll(); err != nil {
			b.Fatal(err)
		}
	}
}
