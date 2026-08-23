package runtime

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
)

/*
TaskHandlerFunc ...
*/
type TaskHandlerFunc[T any] func(task T)

/*
Pool ...
*/
type Pool[T any] struct {
	handlerFunc        TaskHandlerFunc[T]
	idleWorkerLifetime time.Duration
	numShards          int
	maxWorkers         int
	queueSize          int
	shardMinWorkers    int
	shardMaxWorkers    int
	shards             []*poolShard[T]
	notify             chan struct{}
	stopChan           chan struct{}
	doneChan           chan struct{}
	doneOnce           sync.Once
	mutex              sync.Mutex
	started            bool
	stopped            int32

	spawnedWorkers uint64
	_              [56]byte

	waiters uint64
}

type poolShard[T any] struct {
	pool      *Pool[T]
	tqLock    sync.RWMutex
	taskQueue chan T
	workers   int64
}

const defaultIdleWorkerLifetime = time.Second
const maxShards = 128
const defaultQueueSize = 1024
const defaultShardMinWorkers = 2
const defaultShardMaxWorkers = 2048
const defaultNumShardsMin = 2
const defaultNumShardsMax = 48

// defaultNumShards returns GOMAXPROCS/2, clamped to [defaultNumShardsMin, defaultNumShardsMax].
func defaultNumShards() int {
	n := runtime.GOMAXPROCS(0) / 2
	if n < defaultNumShardsMin {
		n = defaultNumShardsMin
	}
	if n > defaultNumShardsMax {
		n = defaultNumShardsMax
	}
	if (n % 2) != 0 {
		n++
	}

	return n
}

/*
NewPool ...
*/
func NewPool[T any](handlerFunc TaskHandlerFunc[T]) *Pool[T] {
	wp := &Pool[T]{
		handlerFunc:        handlerFunc,
		idleWorkerLifetime: defaultIdleWorkerLifetime,
		numShards:          defaultNumShards(),
		maxWorkers:         0,
		queueSize:          defaultQueueSize,
		shardMinWorkers:    defaultShardMinWorkers,
		shardMaxWorkers:    defaultShardMaxWorkers,
	}

	return wp
}

// Sets the maximum number of workers that may exist concurrently.
func (pool *Pool[T]) SetMaxWorkers(n int) {
	if n < 0 {
		n = 0
	}
	pool.maxWorkers = n
}

// Sets the per-shard task queue capacity. Values below 16 are clamped to 16.
func (pool *Pool[T]) SetQueueSize(size int) {
	if size < 16 {
		size = 16
	}
	pool.queueSize = size
}

// Sets the minimum number of workers per shard that are kept alive when idle.
// Also used as the initial worker count per shard at Start().
func (pool *Pool[T]) SetShardMinWorkers(n int) {
	if n < 1 {
		n = 1
	}
	pool.shardMinWorkers = n
}

// Sets the maximum number of workers that may be spawned per shard.
// Acts as a per-shard backpressure cap independent of (and additional to)
// the global SetMaxWorkers cap. Values <= 0 reset to defaultShardMaxWorkers.
func (pool *Pool[T]) SetShardMaxWorkers(n int) {
	if n <= 0 {
		n = defaultShardMaxWorkers
	}
	pool.shardMaxWorkers = n
}

// Sets number of shards. Values <= 0 reset to the runtime-derived default
// (GOMAXPROCS/4, clamped to [defaultNumShardsMin, defaultNumShardsMax]).
func (pool *Pool[T]) SetNumShards(numShards int) {
	if numShards <= 0 {
		numShards = defaultNumShards()
	}
	if numShards > maxShards {
		numShards = maxShards
	}
	pool.numShards = numShards
}

// Sets the idle worker lifetime
func (pool *Pool[T]) SetIdleWorkerLifetime(d time.Duration) {
	pool.idleWorkerLifetime = d
}

// Returns the number of currently spawned workers
func (pool *Pool[T]) GetSpawnedWorkers() int {
	return int(atomic.LoadUint64(&pool.spawnedWorkers))
}

// Returns the number of shards
func (pool *Pool[T]) GetNumShards() int {
	return pool.numShards
}

// Starts the worker pool
func (pool *Pool[T]) Start() {
	pool.mutex.Lock()
	defer pool.mutex.Unlock()

	if pool.started {
		return
	}

	if pool.numShards <= 0 {
		pool.numShards = defaultNumShards()
	}

	pool.notify = make(chan struct{}, 1)
	pool.stopChan = make(chan struct{})
	pool.doneChan = make(chan struct{})
	pool.doneOnce = sync.Once{}

	for i := 0; i < pool.numShards; i++ {
		shard := &poolShard[T]{
			pool:      pool,
			taskQueue: make(chan T, pool.queueSize),
		}
		pool.shards = append(pool.shards, shard)

		// Start initial workers per shard
		for j := 0; j < pool.shardMinWorkers; j++ {
			shard.spawnWorker()
		}
	}

	pool.started = true
}

// Stops the worker pool
func (pool *Pool[T]) Stop() {
	pool.mutex.Lock()
	defer pool.mutex.Unlock()

	if !pool.started || atomic.LoadInt32(&pool.stopped) != 0 {
		return
	}

	atomic.StoreInt32(&pool.stopped, 1)
	close(pool.stopChan)

	// Close each shard's taskQueue under tqLock. Lock waits for any in-flight
	// dispatcher's RLock to release, so no send can race the close. Late
	// dispatchers acquire RLock after this point and see pool.stopped == 1, so
	// they bail with ErrPoolStopped before touching the channel. Workers
	// drain buffered tasks and then see !ok on their next receive and exit.
	for _, shard := range pool.shards {
		shard.tqLock.Lock()
		close(shard.taskQueue)
		shard.tqLock.Unlock()
	}
}

// Stops the worker pool and blocks until all workers have exited.
func (pool *Pool[T]) StopAndWait() {
	pool.Stop()
	<-pool.doneChan
}

// Stops the worker pool and waits up to timeout for all workers to exit.
// Returns true if all workers exited, false on timeout.
func (pool *Pool[T]) StopWithTimeout(timeout time.Duration) bool {
	pool.Stop()
	select {
	case <-pool.doneChan:
		return true
	case <-time.After(timeout):
		return false
	}
}

// Adds a new task
func (pool *Pool[T]) AddTask(task T) error {
	if !pool.started {
		return errors.New("worker pool must be started first")
	}

	if atomic.LoadInt32(&pool.stopped) != 0 {
		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"pool: stopped",
			nil,
		))
	}

	shard := pool.shards[randInt()%pool.numShards]
	return shard.dispatch(task)
}

// Adds a new task and blocks until submitted
func (pool *Pool[T]) AddTaskWithBlocking(task T) (err error) {
	if err = pool.AddTask(task); err == nil || !errnie.IsTooManyRequests(err) {
		return err
	}

	atomic.AddUint64(&pool.waiters, 1)

	for {
		if err = pool.AddTask(task); err == nil {
			n := atomic.AddUint64(&pool.waiters, ^uint64(0))
			if n > 0 {
				select {
				case pool.notify <- struct{}{}:
				default:
				}
			}
			return nil
		}

		if !errnie.IsTooManyRequests(err) {
			atomic.AddUint64(&pool.waiters, ^uint64(0))
			return err
		}

		select {
		case <-pool.notify:
		case <-pool.stopChan:
			atomic.AddUint64(&pool.waiters, ^uint64(0))
			return errors.New("worker pool stopped")
		}
	}
}

// dispatch enqueues a task and spawns a worker on visible backlog.
// The RLock fences the entire critical section (both send attempts and the
// spawn calls) against Stop's close of taskQueue. Late dispatchers re-check
// pool.stopped under the lock to close the TOCTOU window between AddTask's
// fast-path check and the actual send. A non-zero len() after a successful
// send means no idle worker grabbed the task directly, so it would have to
// wait — spawn one (capped).
func (shard *poolShard[T]) dispatch(task T) error {
	if len(shard.taskQueue) > 0 {
		//shard.trySpawnWorker()
	}

	shard.tqLock.RLock()

	if atomic.LoadInt32(&shard.pool.stopped) != 0 {
		shard.tqLock.RUnlock()

		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"pool: stopped",
			nil,
		))
	}

	select {
	case shard.taskQueue <- task:
		if len(shard.taskQueue) > 0 {
			shard.trySpawnWorker()
		}

		shard.tqLock.RUnlock()
		return nil
	default:
	}

	// buffer full — spawn and retry once
	shard.trySpawnWorker()

	// retry a non-blocking enqueue; a worker may have drained the buffer after trySpawnWorker.
	select {
	case shard.taskQueue <- task:
		shard.tqLock.RUnlock()
		return nil
	default:
		shard.tqLock.RUnlock()
		return errnie.Error(errnie.Err(
			errnie.TooManyRequests,
			"pool: overloaded",
			nil,
		))
	}
}

// trySpawnWorker attempts to spawn a new worker for this shard, respecting both
// the per-shard cap (shardMaxWorkers) and the global cap (pool.maxWorkers).
// Both bounds are enforced atomically via CAS to prevent TOCTOU over-spawn
// when many dispatchers race the spawn decision.
func (shard *poolShard[T]) trySpawnWorker() bool {
	pool := shard.pool
	shardMax := int64(pool.shardMaxWorkers)

	// Reserve a per-shard slot atomically.
	for {
		cur := atomic.LoadInt64(&shard.workers)

		if cur >= shardMax {
			return false
		}

		if atomic.CompareAndSwapInt64(&shard.workers, cur, cur+1) {
			break
		}
	}

	// Reserve a global slot atomically (or unconditional add when no cap).
	if pool.maxWorkers > 0 {
		for {
			cur := atomic.LoadUint64(&pool.spawnedWorkers)

			if cur >= uint64(pool.maxWorkers) {
				// Roll back the per-shard reservation.
				atomic.AddInt64(&shard.workers, -1)
				return false
			}

			if atomic.CompareAndSwapUint64(&pool.spawnedWorkers, cur, cur+1) {
				break
			}
		}
	} else {
		atomic.AddUint64(&pool.spawnedWorkers, 1)
	}

	go shard.workerLoop()
	return true
}

/*
spawnWorker is used by Start() for initial worker creation; it bypasses
the per-shard cap check (Start owns the bookkeeping itself).
*/
func (shard *poolShard[T]) spawnWorker() {
	atomic.AddUint64(&shard.pool.spawnedWorkers, 1)
	atomic.AddInt64(&shard.workers, 1)
	go shard.workerLoop()
}

// workerLoop is the main worker goroutine. It reads from its shard's
// taskQueue. Workers above the per-shard floor exit after idleWorkerLifetime
// without receiving a task. On Stop, taskQueue is closed: buffered values
// drain first, then receives return !ok and the worker exits.
func (shard *poolShard[T]) workerLoop() {
	pool := shard.pool
	idleTimeout := pool.idleWorkerLifetime
	var idleTimer *time.Timer

	for {
		// Run queued work first; without any timer overhead. A closed channel
		// drains buffered values before returning !ok, so this naturally
		// handles "drain remaining tasks before exiting" on Stop.
		for {
			select {
			case task, ok := <-shard.taskQueue:
				if !ok {
					goto exit
				}
				pool.handlerFunc(task)
			default:
				goto idle
			}
		}

	idle:
		pool.notifyWaiter()

		// Floor workers wait indefinitely to keep the shard warm. Plain
		// chanrecv (the compiler skips selectgo for a single-case receive).
		if atomic.LoadInt64(&shard.workers) <= int64(pool.shardMinWorkers) {
			task, ok := <-shard.taskQueue

			if !ok {
				goto exit
			}

			pool.handlerFunc(task)
			continue
		}

		// Workers above the floor may retire after an idle timeout.
		if idleTimer == nil {
			idleTimer = time.NewTimer(idleTimeout)
		} else {
			idleTimer.Reset(idleTimeout)
		}

		select {
		case task, ok := <-shard.taskQueue:
			if !idleTimer.Stop() {
				// drain stale value (not required for Go 1.23+)
				select {
				case <-idleTimer.C:
				default:
				}
			}

			if !ok {
				goto exit
			}

			pool.handlerFunc(task)
		case <-idleTimer.C:
			for {
				workers := atomic.LoadInt64(&shard.workers)

				if workers <= int64(pool.shardMinWorkers) {
					break
				}

				// Only exit if the decrement keeps the shard at or above its floor.
				if atomic.CompareAndSwapInt64(&shard.workers, workers, workers-1) {
					goto exit2
				}
			}
		}
	}

exit:
	atomic.AddInt64(&shard.workers, -1)
exit2:
	atomic.AddUint64(&pool.spawnedWorkers, ^uint64(0))
	pool.notifyWaiter()

	if atomic.LoadInt32(&pool.stopped) != 0 && atomic.LoadUint64(&pool.spawnedWorkers) == 0 {
		pool.doneOnce.Do(func() { close(pool.doneChan) })
	}
}

func (pool *Pool[T]) notifyWaiter() {
	if atomic.LoadUint64(&pool.waiters) == 0 {
		return
	}

	select {
	case pool.notify <- struct{}{}:
	default:
	}
}
