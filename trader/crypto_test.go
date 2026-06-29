package trader

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

func TestCryptoRunTicksPastFrontendFreezeRange(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := qpool.NewQ[any](ctx, 1, 2, &qpool.Config{
		SchedulingTimeout:  time.Second,
		JobChannelCapacity: 4,
		Scaler:             nil,
	})
	defer pool.Close()

	crypto, err := NewCrypto(ctx, pool, dmt.NewTree(""))
	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}
	defer crypto.Close()

	done := make(chan error, 1)
	go func() {
		done <- crypto.Run()
	}()

	deadline := time.After(4 * time.Second)
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()

	for {
		select {
		case <-poll.C:
			if count := crypto.tick.Load(); count >= 25 {
				cancel()
				if err := <-done; err != nil {
					t.Fatalf("crypto run returned error: %v", err)
				}
				return
			}
		case err := <-done:
			t.Fatalf("crypto run stopped before tick 25: %v", err)
		case <-deadline:
			t.Fatalf("crypto run reached tick %d, want at least 25", crypto.tick.Load())
		}
	}
}
