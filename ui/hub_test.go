package ui

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
)

func TestHubCachesDashboardSnapshot(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	hub, err := NewHub(ctx, pool, nil)

	if err != nil {
		t.Fatalf("new hub: %v", err)
	}

	hub.cacheSnapshot(map[string]any{
		"event": "engine_pulse",
		"seq":   42,
	})
	hub.cacheSnapshot(map[string]any{
		"event":  "candle_bar",
		"symbol": "BTC/EUR",
	})

	frames := hub.dashboardSnapshot()

	if len(frames) != 1 {
		t.Fatalf("expected one snapshot frame, got %d", len(frames))
	}

	if frames[0]["seq"] != 42 {
		t.Fatalf("unexpected snapshot: %+v", frames[0])
	}
}

func TestSubscriptionCommandsHandleSubscribe(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	group := pool.CreateBroadcastGroup("subscriptions", 10*time.Millisecond)
	subscriber := group.Subscribe("test:subscriptions", 8)
	commands := NewSubscriptionCommands(pool)

	commands.HandleCommand([]byte(`{"op":"subscribe","symbols":["BTC/EUR","ETH/EUR"]}`))

	select {
	case value := <-subscriber.Incoming:
		symbols, ok := value.Value.([]string)

		if !ok {
			t.Fatalf("expected []string, got %T", value.Value)
		}

		if len(symbols) != 2 || symbols[0] != "BTC/EUR" || symbols[1] != "ETH/EUR" {
			t.Fatalf("unexpected symbols: %+v", symbols)
		}
	case <-time.After(time.Second):
		t.Fatal("expected subscription command on broadcast group")
	}
}

func TestListenAddr(t *testing.T) {
	addr, ok := ListenAddr(":8765")

	if !ok || addr != "127.0.0.1:8765" {
		t.Fatalf("unexpected listen addr: %q ok=%v", addr, ok)
	}
}
