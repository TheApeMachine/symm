package client

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/symm/replay"
)

func TestReplayDispatchesRegisteredHandlers(t *testing.T) {
	ctx := context.Background()
	publicClient := NewPublicClient(ctx, WithReplay([][]byte{
		[]byte(`{"channel":"trade","type":"update","data":[{"symbol":"BTC/EUR","side":"buy","qty":1,"price":1,"timestamp":"2026-05-23T02:00:00Z"}]}`),
	}, 0))

	received := 0
	publicClient.OnFrame(func(_ context.Context, payload []byte) error {
		if len(payload) == 0 {
			t.Fatal("expected payload")
		}

		received++
		return nil
	})

	if err := publicClient.Connect(); err != nil {
		t.Fatalf("connect replay client: %v", err)
	}

	publicClient.StartReplay()

	deadline := time.Now().Add(time.Second)

	for received == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	if received != 1 {
		t.Fatalf("expected one replay frame, got %d", received)
	}
}

func TestLoadAndReplayFixture(t *testing.T) {
	frames, err := replay.LoadFrames("../../replay/fixtures/sample.jsonl")

	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}

	ctx := context.Background()
	publicClient := NewPublicClient(ctx, WithReplay(frames, 0))
	count := 0
	publicClient.OnFrame(func(_ context.Context, _ []byte) error {
		count++
		return nil
	})

	if err := publicClient.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}

	publicClient.StartReplay()

	deadline := time.Now().Add(2 * time.Second)

	for count < len(frames) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	if count != len(frames) {
		t.Fatalf("expected %d frames, got %d", len(frames), count)
	}
}
