package work

import (
	"context"
	"testing"
)

func TestNewPoolReturnsWorkerPool(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := NewPool(ctx)

	if pool == nil {
		t.Fatal("expected pool")
	}

	minWorkers, maxWorkers := pool.WorkerBounds()

	if minWorkers < 1 || maxWorkers < minWorkers {
		t.Fatalf("unexpected worker bounds min=%d max=%d", minWorkers, maxWorkers)
	}
}
