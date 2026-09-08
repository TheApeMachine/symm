package runtime

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
)

// TaskHandlerFunc[T] is the function the pool invokes for every submitted task.
type TaskHandlerFunc[T any] func(task T)

// poolWorker is a single reusable goroutine with an elastic lifecycle. It parks
// when there is nothing to do and retires itself after an idle period.
type poolWorker struct {
	// wake carries the single signal a submitter sends to hand this parked
	// worker a fresh task. Capacity one keeps the send non-blocking even when
	// the worker is already committed to retiring.
	wake chan struct{}
	// idleTimer is reused across parks to time the scale-down decision.
	idleTimer *time.Timer
}

/*
Pool is a configuration-free elastic worker pool.

It carries no minimum or maximum for shards, workers, or queue depth. The pool
grows and shrinks purely from measured pressure:

  - Scale up: a new worker is spawned exactly when an arriving task finds no
    parked worker to hand it to AND outstanding work (queued plus in-flight)
    exceeds the number of live workers. Spawns therefore track real backlog
    demand instead of a configured ceiling.
  - Scale down: a parked worker that receives no task within idleWorkerLifetime
    retires itself, so employment is proportional to demand and collapses back to
    zero once the queue drains.

The task buffer is unbounded by design: elasticity is expressed through worker
concurrency, not through a cap that would force callers to pick a drop policy.
*/
type Pool[T any] struct {
	handlerFunc        TaskHandlerFunc[T]
	idleWorkerLifetime time.Duration

	workMu sync.Mutex
	// pending is the unbounded backlog awaiting a worker.
	pending []T
	// parked holds LIFO workers that are idle and waiting for a task.
	parked []*poolWorker
	// live is the number of worker goroutines currently spawned.
	live int
	// inflight is the number of workers currently executing a task.
	inflight int

	stopChan chan struct{}
	doneChan chan struct{}
	doneOnce sync.Once

	mutex   sync.Mutex
	started atomic.Bool
	stopped atomic.Bool
}

// NewPool creates a new elastic Pool for the given task handling function. Call
// Start before submitting tasks.
func NewPool[T any](handlerFunc TaskHandlerFunc[T]) *Pool[T] {
	return &Pool[T]{
		handlerFunc:        handlerFunc,
		idleWorkerLifetime: time.Second,
	}
}

// SetIdleWorkerLifetime sets how long a parked worker stays before retiring,
// which governs how quickly pressure from a drained queue scales the pool down.
func (wp *Pool[T]) SetIdleWorkerLifetime(d time.Duration) {
	if d <= 0 {
		d = time.Second
	}

	wp.idleWorkerLifetime = d
}

// GetSpawnedWorkers returns the number of live worker goroutines, which is
// exactly the pool's current auto-scaled pressure level.
func (wp *Pool[T]) GetSpawnedWorkers() int {
	wp.workMu.Lock()
	defer wp.workMu.Unlock()

	return wp.live
}

// Start makes the pool ready to accept tasks. It is idempotent.
func (wp *Pool[T]) Start() {
	wp.mutex.Lock()
	defer wp.mutex.Unlock()

	if wp.started.Load() {
		return
	}

	if wp.idleWorkerLifetime <= 0 {
		wp.idleWorkerLifetime = time.Second
	}

	wp.stopChan = make(chan struct{})
	wp.doneChan = make(chan struct{})
	wp.doneOnce = sync.Once{}
	wp.stopped.Store(false)
	wp.started.Store(true)
}

// Stop stops submission and lets workers drain queued work before exiting.
// Returns once the shutdown signal has been sent.
func (wp *Pool[T]) Stop() {
	if !wp.started.Load() {
		return
	}

	wp.stopped.CompareAndSwap(false, true)
	close(wp.stopChan)

	// If every worker already retired while idle before this Stop, nobody
	// remains to close the shutdown barrier, so do it here.
	wp.workMu.Lock()
	if wp.live == 0 {
		wp.doneOnce.Do(func() { close(wp.doneChan) })
	}
	wp.workMu.Unlock()
}

// StopAndWait stops the pool and blocks until every worker has exited.
func (wp *Pool[T]) StopAndWait() {
	wp.Stop()
	<-wp.doneChan
}

// StopWithTimeout stops the pool and waits up to timeout for all workers to
// exit. Returns true if every worker exited, false on timeout.
func (wp *Pool[T]) StopWithTimeout(timeout time.Duration) bool {
	wp.Stop()

	select {
	case <-wp.doneChan:
		return true
	case <-time.After(timeout):
		return false
	}
}

// AddTask enqueues a task and, when pressure demands it, spawns a worker to
// keep up. The unbounded backlog means a running pool always accepts a task; it
// can only fail while not started or after a Stop.
func (wp *Pool[T]) AddTask(task T) *errnie.ErrnieError {
	wp.workMu.Lock()

	if !wp.started.Load() || wp.stopped.Load() {
		wp.workMu.Unlock()
		return errnie.Err(
			errnie.NotFound,
			"runtime: pool stopped",
			nil,
		)
	}

	wp.pending = append(wp.pending, task)

	// Hand work to a parked worker first: cheaper than spawning a goroutine and
	// the natural scale-down path.
	if parked := len(wp.parked); parked > 0 {
		poolWorker := wp.parked[parked-1]
		wp.parked = wp.parked[:parked-1]
		select {
		case poolWorker.wake <- struct{}{}:
		default:
		}
		wp.workMu.Unlock()
		return nil
	}

	// No parked worker is available. Spawn only when outstanding work is beyond
	// what live workers can already absorb, so transient single-task submits
	// do not balloon concurrency.
	outstanding := len(wp.pending) + wp.inflight
	if outstanding > wp.live {
		wp.live++
		go wp.runWorker(&poolWorker{wake: make(chan struct{}, 1)})
	}

	wp.workMu.Unlock()
	return nil
}

// AddTaskWithBlocking queues task. The unbounded queue admits it immediately;
// it only blocks long enough to respect a concurrent Stop before reporting the
// shutdown as an error.
func (wp *Pool[T]) AddTaskWithBlocking(task T) error {
	if err := wp.AddTask(task); err == nil {
		return nil
	}

	for {
		select {
		case <-wp.stopChan:
			return errors.New("runtime: pool stopped")
		case <-time.After(time.Millisecond):
		}

		if err := wp.AddTask(task); err == nil {
			return nil
		}
	}
}

// fetchTask returns the next task for this worker, or false when the worker
// should exit (pool stopping, or scaled back down after an idle lifetime). It
// parks itself in wp.parked whenever the queue is empty.
func (wp *Pool[T]) fetchTask(poolWorker *poolWorker) (task T, ok bool) {
	wp.workMu.Lock()

	for {
		if len(wp.pending) > 0 {
			task = wp.pending[0]
			wp.pending = wp.pending[1:]
			wp.inflight++
			wp.workMu.Unlock()
			return task, true
		}

		if wp.stopped.Load() {
			wp.removeParked(poolWorker)
			wp.workMu.Unlock()
			return task, false
		}

		// Discard any stale wake left from a task we already fetched, so a
		// future wake cannot shadow a fresh one.
		select {
		case <-poolWorker.wake:
		default:
		}

		wp.parked = append(wp.parked, poolWorker)
		wp.workMu.Unlock()

		if poolWorker.idleTimer == nil {
			poolWorker.idleTimer = time.NewTimer(wp.idleWorkerLifetime)
		} else {
			poolWorker.idleTimer.Reset(wp.idleWorkerLifetime)
		}

		select {
		case <-poolWorker.wake:
			// A submitter claimed this worker; loop and drain the queue.
		case <-wp.stopChan:
			wp.workMu.Lock()
			wp.removeParked(poolWorker)
			wp.workMu.Unlock()
			return task, false
		case <-poolWorker.idleTimer.C:
			wp.workMu.Lock()
			retire := wp.removeParked(poolWorker)
			wp.workMu.Unlock()

			if retire {
				return task, false
			}
			// Claimed between the wake and the timeout: loop and take the work.
		}

		wp.workMu.Lock()
	}
}

// removeParked drops the worker from the parked stack, returning true if it was
// parked there. It returns false when the worker was already handed a task by a
// submitter, in which case it must not retire.
func (wp *Pool[T]) removeParked(poolWorker *poolWorker) bool {
	for index, parkedWorker := range wp.parked {
		if parkedWorker == poolWorker {
			wp.parked = append(wp.parked[:index], wp.parked[index+1:]...)
			return true
		}
	}

	return false
}

// runWorker is the elastic lifecycle of one goroutine: it processes tasks until
// it retires, then releases its liveness slot and, once the pool is fully
// drained and stopped, signals the shutdown barrier.
func (wp *Pool[T]) runWorker(poolWorker *poolWorker) {
	for {
		task, ok := wp.fetchTask(poolWorker)
		if !ok {
			wp.workMu.Lock()
			wp.live--
			if wp.stopped.Load() && wp.live == 0 {
				wp.doneOnce.Do(func() { close(wp.doneChan) })
			}
			wp.workMu.Unlock()
			return
		}

		wp.handlerFunc(task)

		wp.workMu.Lock()
		wp.inflight--
		wp.workMu.Unlock()
	}
}
