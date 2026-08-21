package types

import (
	"sync"
	"sync/atomic"

	"github.com/spf13/viper"
)

/*
poolError is the tiny immutable holder kept behind an atomic pointer so the
first failure a worker observes can be published lock-free.
*/
type poolError struct {
	err error
}

/*
SymbolPool runs per-symbol stage work concurrently across a fixed number of
workers while preserving per-symbol ordering: every occurrence of one symbol is
routed to the same worker by a stable shard, so a symbol's analytical state is
only ever advanced by one goroutine. Unrelated symbols run in parallel, which
is what lets a stage service every symbol instead of serializing behind one
slow market.

No mutex protects pool data. Errors are published with an atomic
compare-and-swap, per-shard work is a channel, shutdown is a one-shot done
signal, and worker exit is a WaitGroup. Close is idempotent and Submit is safe
to race with Close: a submit that loses the shutdown race simply drops the
symbol, because the stage is tearing down and the work is gone either way.
*/
type SymbolPool struct {
	workers int
	queues  []chan func()
	group   sync.WaitGroup
	err     atomic.Pointer[poolError]
	done    chan struct{}
	closer  sync.Once
}

/*
NewSymbolPool creates a symbol-sharded worker pool. A zero or negative worker
count defaults to one serial worker so tests and fixtures stay deterministic.
*/
func NewSymbolPool(workers int) *SymbolPool {
	if workers < 1 {
		workers = 1
	}

	pool := &SymbolPool{
		workers: workers,
		queues:  make([]chan func(), workers),
		done:    make(chan struct{}),
	}

	for index := range workers {
		queue := make(chan func(), 1024)
		pool.queues[index] = queue
		pool.group.Add(1)

		go func() {
			defer pool.group.Done()

			for {
				select {
				case <-pool.done:
					return
				case task := <-queue:
					task()
				}
			}
		}()
	}

	return pool
}

/*
Submit queues one symbol's work on its shard. The caller owns the closure's
error capture. Submit is allocation-light and never blocks a producer beyond
its shard's queue capacity; it returns without publishing once the pool has
been closed.
*/
func (pool *SymbolPool) Submit(symbol string, task func()) {
	if pool == nil || symbol == "" || task == nil {
		return
	}

	shard := stableShard(symbol, pool.workers)

	select {
	case <-pool.done:
		return
	case pool.queues[shard] <- task:
	}
}

/*
CaptureError records the first stage failure observed by any worker. Later
failures are ignored because the first one is the one that halts the system.
*/
func (pool *SymbolPool) CaptureError(err error) {
	if pool == nil || err == nil {
		return
	}

	pool.err.CompareAndSwap(nil, &poolError{err: err})
}

/*
Error returns the first failure captured by a worker, if any.
*/
func (pool *SymbolPool) Error() error {
	if pool == nil {
		return nil
	}

	if first := pool.err.Load(); first != nil {
		return first.err
	}

	return nil
}

/*
Close flushes and releases every worker exactly once. It is safe to call from
any goroutine and may race with Submit; in-flight workers finish their current
symbol before the stage completes teardown.
*/
func (pool *SymbolPool) Close() {
	if pool == nil {
		return
	}

	pool.closer.Do(func() {
		close(pool.done)
		pool.group.Wait()
	})
}

/*
ShardWorkers returns the configured per-stage symbol concurrency. Tests and
fixtures that never load a config file fall back to one serial worker so their
ordering stays deterministic.
*/
func ShardWorkers() int {
	workers := viper.GetInt("system.streaming.symbol_shards")

	if workers < 1 {
		return 1
	}

	return workers
}

/*
stableShard maps one symbol to a stable worker index using FNV-1a so routing
does not depend on map iteration order or goroutine scheduling.
*/
func stableShard(symbol string, workers int) int {
	hash := uint64(1469598103934665603)

	for index := range len(symbol) {
		hash ^= uint64(symbol[index])
		hash *= 1099511628211
	}

	return int(hash % uint64(workers))
}
