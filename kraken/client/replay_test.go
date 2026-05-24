package client

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/theapemachine/symm/replay"
)

func TestReplayDispatchesRegisteredHandlers(t *testing.T) {
	ctx := context.Background()
	publicClient := NewPublicClient(ctx, WithReplay([][]byte{
		[]byte(`{"channel":"trade","type":"update","data":[{"symbol":"BTC/EUR","side":"buy","qty":1,"price":1,"timestamp":"2026-05-23T02:00:00Z"}]}`),
	}, 0))

	var received atomic.Int32
	publicClient.OnFrame(func(_ context.Context, payload []byte) error {
		if len(payload) == 0 {
			t.Fatal("expected payload")
		}

		received.Add(1)

		return nil
	})

	if err := publicClient.Connect(); err != nil {
		t.Fatalf("connect replay client: %v", err)
	}

	publicClient.StartReplay()

	deadline := time.Now().Add(time.Second)

	for received.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	if received.Load() != 1 {
		t.Fatalf("expected one replay frame, got %d", received.Load())
	}
}

func TestLoadAndReplayFixture(t *testing.T) {
	frames, err := replay.LoadFrames("../../replay/fixtures/sample.jsonl")

	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}

	ctx := context.Background()
	publicClient := NewPublicClient(ctx, WithReplay(frames, 0))
	var count atomic.Int32
	publicClient.OnFrame(func(_ context.Context, _ []byte) error {
		count.Add(1)

		return nil
	})

	if err := publicClient.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}

	publicClient.StartReplay()

	deadline := time.Now().Add(2 * time.Second)

	for int(count.Load()) < len(frames) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	if int(count.Load()) != len(frames) {
		t.Fatalf("expected %d frames, got %d", len(frames), count.Load())
	}
}
