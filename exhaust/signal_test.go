package exhaust

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trade"
)

func TestExhaustPublishPulseEveryTick(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewExhaust(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	for index, bidDepth := range []float64{100, 98, 96, 94, 92, 90, 88, 86, 84, 82, 80, 20, 18, 16, 14} {
		spreadBPS := 10.0

		if index >= 11 {
			spreadBPS = 40
		}

		signal.history.observe(
			"ALT/EUR",
			bidDepth,
			90,
			bidDepth+90,
			spreadBPS,
			0.9-float64(index)*0.05,
			0.6-float64(index)*0.04,
			10,
		)
	}

	exits := signal.broadcasts["exits"].Subscribe("test:exhaust", 8)

	pool.CreateBroadcastGroup("tick", 0).Send(&qpool.QValue[any]{
		Value: market.TickerRow{Symbol: "ALT/EUR", Last: 10},
	})

	if err := signal.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}

	select {
	case value := <-exits.Incoming:
		payload, ok := value.Value.(engine.Exit)

		if !ok || payload.Symbol != "ALT/EUR" {
			t.Fatalf("expected exit payload, got %v", value.Value)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for exhaust exit publish")
	}
}

func TestExhaustTickObservesBook(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewExhaust(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	pool.CreateBroadcastGroup("book", 0).Send(&qpool.QValue[any]{
		Value: market.BookLevelsDelta{
			Symbol: "ALT/EUR",
			Bids:   []market.BookLevel{{Price: 10, Volume: 100}},
			Asks:   []market.BookLevel{{Price: 10.1, Volume: 90}},
		},
	})

	if err := signal.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}

	snapshot, ok := signal.history.snapshot("ALT/EUR")

	if !ok || snapshot.bidDepths.Len() == 0 {
		t.Fatal("expected book history after tick")
	}
}

func TestExhaustTickObservesTrade(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewExhaust(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	pool.CreateBroadcastGroup("trade", 0).Send(&qpool.QValue[any]{
		Value: trade.Data{Symbol: "ALT/EUR", Side: "buy", Price: 10},
	})

	if err := signal.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}

	snapshot, ok := signal.history.snapshot("ALT/EUR")

	if !ok || snapshot.pressures.Len() == 0 {
		t.Fatal("expected trade pressure after tick")
	}
}

func BenchmarkExhaustTickDefault(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	signal := NewExhaust(ctx, pool)
	defer signal.Close()

	for index := range 24 {
		signal.history.observe(
			"ALT/EUR",
			float64(100-index*3),
			90,
			190,
			10,
			0.8-float64(index)*0.03,
			0.4,
			10,
		)
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = signal.Tick()
	}
}
