package client

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
)

func TestPublicClientReplayRoutesTicker(t *testing.T) {
	fixture := filepath.Join("..", "..", "replay", "fixtures", "sample.jsonl")

	if _, err := os.Stat(fixture); err != nil {
		t.Fatal(err)
	}

	config.System.ReplayFile = fixture
	config.System.ReplayPace = 0
	t.Cleanup(func() {
		config.System.ReplayFile = ""
		config.System.ReplayPace = 50 * time.Millisecond
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	tick := pool.CreateBroadcastGroup("tick", 10*time.Millisecond)
	subscriber := tick.Subscribe("test:replay:tick", 8)

	publicClient := NewPublicClient(ctx, pool, "")

	if err := publicClient.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)

	for time.Now().Before(deadline) {
		select {
		case value := <-subscriber.Incoming:
			row, ok := value.Value.(market.TickerRow)

			if !ok {
				t.Fatalf("expected ticker row, got %T", value.Value)
			}

			if row.Symbol != "BTC/EUR" || row.Last != 50000 {
				t.Fatalf("unexpected ticker: %+v", row)
			}

			return
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	t.Fatal("expected ticker from replay fixture")
}
