package cmd

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

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

	if err := booter.AddSystems(first, second); err != nil {
		t.Fatalf("add systems: %v", err)
	}

	go func() {
		_ = booter.Boot()
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	if first.ticks.Load() < 1 {
		t.Fatalf("expected first system ticked, got %d", first.ticks.Load())
	}

	if second.ticks.Load() < 1 {
		t.Fatalf("expected second system ticked, got %d", second.ticks.Load())
	}
}

// failingSystem returns an error from Tick and records whether Close was
// invoked. We use it to prove the booter shuts down the peer system once one
// system's Tick exits.
type failingSystem struct {
	tickErr error
	closed  atomic.Int64
	done    chan struct{}
}

func (system *failingSystem) Start() error        { return nil }
func (system *failingSystem) State() engine.State { return engine.READY }
func (system *failingSystem) Tick() error         { return system.tickErr }
func (system *failingSystem) Close() error {
	system.closed.Add(1)
	if system.done != nil {
		select {
		case <-system.done:
		default:
			close(system.done)
		}
	}
	return nil
}

type blockingSystem struct {
	wait   chan struct{}
	closed atomic.Int64
}

func (system *blockingSystem) Start() error        { return nil }
func (system *blockingSystem) State() engine.State { return engine.READY }
func (system *blockingSystem) Tick() error {
	<-system.wait
	return nil
}
func (system *blockingSystem) Close() error {
	if system.closed.Add(1) == 1 {
		close(system.wait)
	}
	return nil
}

// TestBooterShutsDownPeersOnTickError proves that when one system's Tick
// returns, every other registered system has Close called on it so its
// internal context cancel runs.
func TestBooterShutsDownPeersOnTickError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	failer := &failingSystem{tickErr: context.DeadlineExceeded}
	peer := &blockingSystem{wait: make(chan struct{})}

	booter, err := NewBooter(ctx, pool)
	if err != nil {
		t.Fatalf("new booter: %v", err)
	}

	if err := booter.AddSystems(failer, peer); err != nil {
		t.Fatalf("add systems: %v", err)
	}

	bootDone := make(chan struct{})
	go func() {
		_ = booter.Boot()
		close(bootDone)
	}()

	select {
	case <-bootDone:
	case <-time.After(2 * time.Second):
		t.Fatal("booter did not shut down peers after a system's Tick returned")
	}

	if peer.closed.Load() == 0 {
		t.Fatal("expected blocking peer to receive Close after sibling failed")
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
