package cmd

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
)

type tickSystem struct {
	ticks atomic.Int64
}

func (system *tickSystem) Start() error { return nil }

func (system *tickSystem) State() engine.State { return engine.READY }

func (system *tickSystem) Close() error { return nil }

func (system *tickSystem) Tick() error {
	system.ticks.Add(1)
	return nil
}

func TestBooterConcurrentSystemTicks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := qpool.NewQ(ctx, 2, 8, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	first := &tickSystem{}
	second := &tickSystem{}

	booter, err := NewBooter(ctx, pool)

	if err != nil {
		t.Fatalf("new booter: %v", err)
	}

	booter.AddSystems(first, second)

	go func() {
		_ = booter.Boot()
	}()

	cancel()

	if first.ticks.Load() < 2 {
		t.Fatalf("expected concurrent ticks on first system, got %d", first.ticks.Load())
	}

	if second.ticks.Load() < 2 {
		t.Fatalf("expected concurrent ticks on second system, got %d", second.ticks.Load())
	}
}

func BenchmarkBooterWaitRescore(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	b.Cleanup(cancel)

	pool := qpool.NewQ(ctx, 1, 1, qpool.NewConfig())
	b.Cleanup(func() { pool.Close() })

	booter, err := NewBooter(ctx, pool)

	if err != nil {
		b.Fatalf("new booter: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = booter.Boot()
	}
}
