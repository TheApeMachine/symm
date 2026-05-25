package exhaust

import (
	"context"
	"testing"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trade"
)

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
