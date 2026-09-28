/*
Package pool provides an elastic worker pool as a streaming Primitive.

The pool carries no minimum or maximum for workers or queue depth. It grows and
shrinks purely from measured pressure:

  - Scale up: a new worker is spawned exactly when an arriving task finds no
    parked worker to hand it to AND outstanding work (queued plus in-flight)
    exceeds the number of live workers. Spawns therefore track real backlog
    demand instead of a configured ceiling.
  - Scale down: a parked worker that receives no task within idleWorkerLifetime
    retires itself, so employment is proportional to demand and collapses back
    to zero once the queue drains.

The task buffer is unbounded by design: elasticity is expressed through worker
concurrency, not through a cap that would force callers to pick a drop policy.

On the wire, Next receives *T task pointers. The handler mutates the pointed-to
value in place; the pool yields the same pointer back only after the handler
has completed, which is how results travel the wire.
*/
package pool

import (
	"errors"
	"fmt"
	"iter"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
TaskHandlerFunc is the function the pool invokes for every submitted task. It
receives the task pointer from the wire and mutates it in place.
*/
type TaskHandlerFunc[T any] func(task *T)

/*
sink is the completion path of one Next run: the channel completed task
pointers travel, the channel closed when the run's consumer walks away early,
the channel closed when the input run has been fully submitted, and the
waitgroup counting tasks still owed a completion.
*/
type sink struct {
	completions chan unsafe.Pointer
	abandoned   chan struct{}
	inputDone   chan struct{}
	wg          sync.WaitGroup
}

/*
poolTask is one queued unit: the wire pointer and the run it must report to.
*/
type poolTask[T any] struct {
	value *T
	sink  *sink
}

/*
poolWorker is a single reusable goroutine with an elastic lifecycle. It parks
when there is nothing to do and retires itself after an idle period.
*/
type poolWorker struct {
	// wake carries the single signal a submitter sends to hand this parked
	// worker a fresh task. Capacity one keeps the send non-blocking even when
	// the worker is already committed to retiring.
	wake chan struct{}
	// idleTimer is reused across parks to time the scale-down decision.
	idleTimer *time.Timer
}

/*
Pool is a configuration-free elastic worker pool Primitive. Its stream is its
lifetime: the pool starts on the first Next, submits arriving task pointers to
its elastic workers, yields each pointer after its handler completes, and
drains every queued task before stopping when the input run ends.
*/
type Pool[T any] struct {
	err         error
	handlerFunc TaskHandlerFunc[T]
	lifetime    time.Duration

	gate atomic.Int64
	// pending is the unbounded backlog awaiting a worker.
	pending []*poolTask[T]
	// parked holds LIFO workers that are idle and waiting for a task.
	parked []*poolWorker
	// live is the number of worker goroutines currently spawned.
	live atomic.Int64
	// inflight is the number of workers currently executing a task.
	inflight atomic.Int64

	stopChan chan struct{}
	doneChan chan struct{}
	doneOnce sync.Once

	started atomic.Bool
	stopped atomic.Bool
}

const poolWriterMask = 1 << 30

func (op *Pool[T]) lock() {
	for {
		value := op.gate.Load()

		if value == 0 && op.gate.CompareAndSwap(0, -poolWriterMask) {
			return
		}

		runtime.Gosched()
	}
}

func (op *Pool[T]) unlock() {
	op.gate.Add(poolWriterMask)
}

/*
NewPool creates an elastic pool Primitive that runs the handler for every
arriving task pointer and retires parked workers after idleWorkerLifetime. A
non-positive lifetime is recorded as a domain failure and every stream over
the pool yields nothing.
*/
func NewPool[T any](
	handlerFunc TaskHandlerFunc[T], idleWorkerLifetime time.Duration,
) core.Primitive {
	if idleWorkerLifetime <= 0 {
		return &Pool[T]{
			err: fmt.Errorf("%w: pool: idle worker lifetime must be positive", core.ErrDomain),
		}
	}

	return &Pool[T]{
		handlerFunc: handlerFunc,
		lifetime:    idleWorkerLifetime,
		stopChan:    make(chan struct{}),
		doneChan:    make(chan struct{}),
	}
}

/*
Next receives *T task pointers, executes them on elastic workers, and yields
each pointer after its handler completes. Submission runs ahead of delivery:
between arrivals every completion that is already ready is handed over
without waiting, so bursts create real backlog and scale workers up. When the
input run ends the pool drains every queued task, delivers every completion,
then stops. Early consumer termination abandons the remaining deliveries
without dropping the workers' in-flight mutations.
*/
func (op *Pool[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	if op.err != nil {
		return func(yield func(unsafe.Pointer) bool) {}
	}

	return func(yield func(unsafe.Pointer) bool) {
		op.start()

		current := in

		if current == nil {
			current = func(yield func(unsafe.Pointer) bool) {}
		}

		run := &sink{
			completions: make(chan unsafe.Pointer),
			abandoned:   make(chan struct{}),
			inputDone:   make(chan struct{}),
		}

		submissionDone := sync.OnceFunc(func() { close(run.inputDone) })

		defer close(run.abandoned)
		defer submissionDone()

		go func() {
			<-run.inputDone
			run.wg.Wait()
			close(run.completions)
			op.stop()
		}()

		for arriving := range current {
			if !op.submit((*T)(arriving), run) {
				break
			}

			if !op.drainReady(run, yield) {
				return
			}
		}

		submissionDone()

		for ptr := range run.completions {
			if !yield(ptr) {
				return
			}
		}
	}
}

/*
drainReady hands over every completion that is already waiting, without
blocking on work that is still in flight.
*/
func (op *Pool[T]) drainReady(run *sink, yield func(unsafe.Pointer) bool) bool {
	for {
		select {
		case ptr, ok := <-run.completions:

			if !ok {
				return true
			}

			if !yield(ptr) {
				return false
			}
		default:
			return true
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *Pool[T]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
start makes the pool ready to accept tasks. It is idempotent.
*/
func (op *Pool[T]) start() {
	if !op.started.CompareAndSwap(false, true) {
		return
	}
}

/*
stop stops submission and lets workers drain queued work before exiting. It is
invoked once the stream has fully drained; if every worker already retired
while idle, nobody remains to close the shutdown barrier, so it is done here.
*/
func (op *Pool[T]) stop() {
	if !op.started.Load() {
		return
	}

	if !op.stopped.CompareAndSwap(false, true) {
		return
	}

	close(op.stopChan)

	if op.live.Load() == 0 {
		op.doneOnce.Do(func() { close(op.doneChan) })
	}
}

/*
submit enqueues a task and, when pressure demands it, spawns a worker to keep
up. The unbounded backlog means a running pool always accepts a task; it can
only fail once the pool has stopped, which is recorded and ends the stream.
*/
func (op *Pool[T]) submit(value *T, run *sink) bool {
	if !op.started.Load() || op.stopped.Load() {
		op.Error(fmt.Errorf("%w: pool: stopped", core.ErrDomain))

		return false
	}

	run.wg.Add(1)

	op.lock()

	if op.stopped.Load() {
		op.unlock()
		run.wg.Done()
		op.Error(fmt.Errorf("%w: pool: stopped", core.ErrDomain))

		return false
	}

	op.pending = append(op.pending, &poolTask[T]{value: value, sink: run})

	// Hand work to a parked worker first: cheaper than spawning a goroutine
	// and the natural scale-down path.
	if parkedCount := len(op.parked); parkedCount > 0 {
		worker := op.parked[parkedCount-1]
		op.parked = op.parked[:parkedCount-1]
		select {
		case worker.wake <- struct{}{}:
		default:
		}
		op.unlock()

		return true
	}

	// No parked worker is available. Spawn only when outstanding work is
	// beyond what live workers can already absorb, so transient single-task
	// submits do not balloon concurrency.
	outstanding := len(op.pending) + int(op.inflight.Load())

	if outstanding > int(op.live.Load()) {
		op.live.Add(1)
		go op.runWorker(&poolWorker{wake: make(chan struct{}, 1)})
	}

	op.unlock()

	return true
}

/*
fetchTask returns the next task for this worker, or false when the worker
should exit (pool stopping, or scaled back down after an idle lifetime). It
parks itself in op.parked whenever the queue is empty.
*/
func (op *Pool[T]) fetchTask(worker *poolWorker) (task *poolTask[T], ok bool) {
	op.lock()

	for {
		if len(op.pending) > 0 {
			task = op.pending[0]
			op.pending = op.pending[1:]
			op.inflight.Add(1)
			op.unlock()

			return task, true
		}

		if op.stopped.Load() {
			op.removeParked(worker)
			op.unlock()

			return task, false
		}

		// Discard any stale wake left from a task we already fetched, so a
		// future wake cannot shadow a fresh one.
		select {
		case <-worker.wake:
		default:
		}

		op.parked = append(op.parked, worker)
		op.unlock()

		if worker.idleTimer == nil {
			worker.idleTimer = time.NewTimer(op.lifetime)
		} else {
			worker.idleTimer.Reset(op.lifetime)
		}

		select {
		case <-worker.wake:
			// A submitter claimed this worker; loop and drain the queue.
		case <-op.stopChan:
			op.lock()
			op.removeParked(worker)
			op.unlock()

			return task, false
		case <-worker.idleTimer.C:
			op.lock()
			retire := op.removeParked(worker)
			op.unlock()

			if retire {
				return task, false
			}
			// Claimed between the wake and the timeout: loop and take the work.
		}

		op.lock()
	}
}

/*
removeParked drops the worker from the parked stack, returning true if it was
parked there. It returns false when the worker was already handed a task by a
submitter, in which case it must not retire.
*/
func (op *Pool[T]) removeParked(worker *poolWorker) bool {
	for index, parkedWorker := range op.parked {
		if parkedWorker == worker {
			op.parked = append(op.parked[:index], op.parked[index+1:]...)

			return true
		}
	}

	return false
}

/*
runWorker is the elastic lifecycle of one goroutine: it processes tasks until
it retires, then releases its liveness slot and, once the pool is fully drained
and stopped, signals the shutdown barrier.
*/
func (op *Pool[T]) runWorker(worker *poolWorker) {
	for {
		task, ok := op.fetchTask(worker)

		if !ok {
			if op.live.Add(-1) == 0 && op.stopped.Load() {
				op.doneOnce.Do(func() { close(op.doneChan) })
			}

			return
		}

		op.handlerFunc(task.value)
		op.inflight.Add(-1)

		select {
		case task.sink.completions <- unsafe.Pointer(task.value):
		case <-task.sink.abandoned:
		}

		task.sink.wg.Done()
	}
}
