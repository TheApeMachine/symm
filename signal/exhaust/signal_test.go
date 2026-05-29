package exhaust

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trade"
)

func startExhaustTick(t *testing.T, exhaust *Exhaust) {
	t.Helper()

	done := make(chan struct{})

	go func() {
		defer close(done)

		if err := exhaust.Tick(); err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("exhaust tick: %v", err)
		}
	}()

	t.Cleanup(func() {
		_ = exhaust.Close()

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for exhaust tick to close")
		}
	})
}

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
	startExhaustTick(t, signal)

	pool.CreateBroadcastGroup("tick", 0).Send(&qpool.QValue[any]{
		Value: market.TickerRow{Symbol: "ALT/EUR", Last: 10},
	})

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
	startExhaustTick(t, signal)

	pool.CreateBroadcastGroup("book", 0).Send(&qpool.QValue[any]{
		Value: market.BookLevelsDelta{
			Symbol: "ALT/EUR",
			Bids:   []market.BookLevel{{Price: 10, Volume: 100}},
			Asks:   []market.BookLevel{{Price: 10.1, Volume: 90}},
		},
	})

	deadline := time.Now().Add(time.Second)

	for time.Now().Before(deadline) {
		snapshot, ok := signal.history.snapshot("ALT/EUR")

		if ok && snapshot.bidDepths.Len() > 0 {
			return
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatal("expected book history after tick")
}

func TestExhaustTickObservesTrade(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewExhaust(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })
	startExhaustTick(t, signal)

	pool.CreateBroadcastGroup("trade", 0).Send(&qpool.QValue[any]{
		Value: trade.Data{Symbol: "ALT/EUR", Side: "buy", Price: 10},
	})

	deadline := time.Now().Add(time.Second)

	for time.Now().Before(deadline) {
		snapshot, ok := signal.history.snapshot("ALT/EUR")

		if ok && snapshot.pressures.Len() > 0 {
			return
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatal("expected trade pressure after tick")
}

func TestExhaustTickDrainsEachChannelGoroutine(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewExhaust(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	signal.mu.Lock()
	locked := true

	defer func() {
		if locked {
			signal.mu.Unlock()
		}
	}()

	startExhaustTick(t, signal)

	signal.subscribers["book"].Incoming <- &qpool.QValue[any]{
		Value: market.BookLevelsDelta{
			Symbol: "ALT/EUR",
			Bids:   []market.BookLevel{{Price: 10, Volume: 100}},
			Asks:   []market.BookLevel{{Price: 10.1, Volume: 90}},
		},
	}
	signal.subscribers["trade"].Incoming <- &qpool.QValue[any]{
		Value: trade.Data{Symbol: "ALT/EUR", Side: "buy", Price: 10},
	}
	signal.subscribers["tick"].Incoming <- &qpool.QValue[any]{
		Value: market.TickerRow{Symbol: "ALT/EUR", Last: 10},
	}

	deadline := time.Now().Add(time.Second)

	for time.Now().Before(deadline) {
		if len(signal.subscribers["book"].Incoming) == 0 &&
			len(signal.subscribers["trade"].Incoming) == 0 &&
			len(signal.subscribers["tick"].Incoming) == 0 {
			signal.mu.Unlock()
			locked = false
			return
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatal("expected every channel to drain while handlers waited on shared state")
}

func BenchmarkExhaustPublishPulse(b *testing.B) {
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
		signal.publishPulse()
	}
}

func BenchmarkExhaustTickConcurrentChannels(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	signal := NewExhaust(ctx, pool)
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = signal.Tick()
	}()

	defer func() {
		_ = signal.Close()

		select {
		case <-done:
		case <-time.After(time.Second):
			b.Fatal("timed out waiting for exhaust tick to close")
		}
	}()

	book := &qpool.QValue[any]{
		Value: market.BookLevelsDelta{
			Symbol: "ALT/EUR",
			Bids:   []market.BookLevel{{Price: 10, Volume: 100}},
			Asks:   []market.BookLevel{{Price: 10.1, Volume: 90}},
		},
	}
	tradeTick := &qpool.QValue[any]{
		Value: trade.Data{Symbol: "ALT/EUR", Side: "buy", Price: 10},
	}
	priceTick := &qpool.QValue[any]{
		Value: market.TickerRow{Symbol: "ALT/EUR", Last: 10},
	}

	b.ReportAllocs()

	for b.Loop() {
		signal.subscribers["book"].Incoming <- book
		signal.subscribers["trade"].Incoming <- tradeTick
		signal.subscribers["tick"].Incoming <- priceTick
	}
}
