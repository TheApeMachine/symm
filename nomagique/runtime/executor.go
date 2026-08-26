package runtime

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

/*
Work represents one pending unit of computation submitted to the keyed executor.
*/
type Work struct {
	runtime     *SubscriberRuntime
	key         string
	value       any
	submittedAt time.Time
}

/*
keyIdentity identifies a unique (subscriber, key) execution stream.
*/
type keyIdentity struct {
	runtimeID uint32
	key       string
}

/*
keyQueue maintains the serial queue of pending work for one (subscriber, key) stream.
*/
type keyQueue struct {
	mu        sync.Mutex
	runtimeID uint32
	key       string
	pending   []Work
	head      int
	scheduled bool
}

func (queue *keyQueue) push(work Work) {
	queue.pending = append(queue.pending, work)
}

func (queue *keyQueue) pop() (Work, bool) {
	if queue.head >= len(queue.pending) {
		queue.pending = queue.pending[:0]
		queue.head = 0
		return Work{}, false
	}

	work := queue.pending[queue.head]
	queue.head++

	if queue.head >= len(queue.pending) {
		queue.pending = queue.pending[:0]
		queue.head = 0
	}

	return work, true
}

func (queue *keyQueue) len() int {
	return len(queue.pending) - queue.head
}

/*
KeyedExecutor is the Workspace-wide bounded CPU scheduler ensuring same-key serial
and different-key parallel execution across runtime.GOMAXPROCS(0) worker goroutines.
*/
type KeyedExecutor struct {
	ctx        context.Context
	cancel     context.CancelFunc
	workers    int
	readyQueue chan *keyQueue
	queues     sync.Map
	pending    atomic.Int64
	activeKeys atomic.Int64
	doneGroup  sync.WaitGroup
}

/*
NewKeyedExecutor constructs a KeyedExecutor with maxWorkers. If maxWorkers <= 0,
it defaults to runtime.GOMAXPROCS(0).
*/
func NewKeyedExecutor(ctx context.Context, maxWorkers int) *KeyedExecutor {
	if ctx == nil {
		ctx = context.Background()
	}

	executorCtx, cancel := context.WithCancel(ctx)
	workerCount := maxWorkers

	if workerCount <= 0 {
		workerCount = runtime.GOMAXPROCS(0)
	}

	if workerCount <= 0 {
		workerCount = 1
	}

	executor := &KeyedExecutor{
		ctx:        executorCtx,
		cancel:     cancel,
		workers:    workerCount,
		readyQueue: make(chan *keyQueue, subscriberCapacity),
	}

	for workerIndex := 0; workerIndex < workerCount; workerIndex++ {
		executor.doneGroup.Add(1)
		go executor.workerLoop()
	}

	return executor
}

func (executor *KeyedExecutor) Close() error {
	executor.cancel()
	executor.doneGroup.Wait()
	return nil
}

func (executor *KeyedExecutor) Pending() int64 {
	return executor.pending.Load()
}

func (executor *KeyedExecutor) ActiveKeys() int64 {
	return executor.activeKeys.Load()
}

func (executor *KeyedExecutor) Workers() int {
	return executor.workers
}

func (executor *KeyedExecutor) Submit(subscriberRuntime *SubscriberRuntime, key string, value any) {
	if subscriberRuntime == nil {
		return
	}

	identity := keyIdentity{
		runtimeID: subscriberRuntime.id,
		key:       key,
	}

	stored, found := executor.queues.Load(identity)
	var queue *keyQueue

	if found {
		queue = stored.(*keyQueue)
	}

	if !found {
		candidate := &keyQueue{
			runtimeID: subscriberRuntime.id,
			key:       key,
			pending:   make([]Work, 0, 16),
		}
		actual, _ := executor.queues.LoadOrStore(identity, candidate)
		queue = actual.(*keyQueue)
	}

	executor.pending.Add(1)
	subscriberRuntime.executorPending.Add(1)

	queue.mu.Lock()
	queue.push(Work{
		runtime:     subscriberRuntime,
		key:         key,
		value:       value,
		submittedAt: time.Now(),
	})

	if !queue.scheduled {
		queue.scheduled = true
		queue.mu.Unlock()
		executor.activeKeys.Add(1)
		executor.readyQueue <- queue
		return
	}

	queue.mu.Unlock()
}

func (executor *KeyedExecutor) workerLoop() {
	defer executor.doneGroup.Done()

	for {
		select {
		case <-executor.ctx.Done():
			return
		case queue, ok := <-executor.readyQueue:
			if !ok {
				return
			}

			executor.processQueue(queue)
		}
	}
}

func (executor *KeyedExecutor) processQueue(queue *keyQueue) {
	queue.mu.Lock()
	work, ok := queue.pop()

	if !ok {
		queue.scheduled = false
		queue.mu.Unlock()
		executor.activeKeys.Add(-1)
		return
	}

	queue.mu.Unlock()

	started := time.Now()
	waitTime := started.Sub(work.submittedAt)
	work.runtime.queueWaitNanos.Add(waitTime.Nanoseconds())

	defer func() {
		duration := time.Since(started)
		work.runtime.stepDurationNanos.Add(duration.Nanoseconds())
		work.runtime.stepCount.Add(1)
		work.runtime.completedEvents.Add(1)
		work.runtime.executorPending.Add(-1)
		executor.pending.Add(-1)

		work.runtime.inFlight.Add(-1)
		work.runtime.workspace.inFlight.Add(-1)

		queue.mu.Lock()
		if queue.len() > 0 {
			queue.mu.Unlock()
			executor.readyQueue <- queue
			return
		}

		queue.scheduled = false
		queue.mu.Unlock()
		executor.activeKeys.Add(-1)
	}()

	work.runtime.process(work.value)
}
