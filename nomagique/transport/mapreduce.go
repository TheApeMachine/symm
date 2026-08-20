package transport

import (
	"context"
	"iter"
	"sync"
	"sync/atomic"

	"golang.design/x/lockfree/lf"
)

type consumer[T any] struct {
	queue       *lf.Queue[T]
	ready       chan struct{}
	previous    T
	hasPrevious bool
	limit       uint64
	queued      atomic.Uint64
	overflowed  atomic.Bool
	scheduled   atomic.Bool
	stageMu     sync.Mutex
}

/*
MapReduce fans each pushed value to independently drained consumer queues.
Consumer lookup is O(1), registration is safe while producers are running, and
each consumer owns a wake channel so idle readers sleep instead of polling.
*/
type MapReduce[T any] struct {
	consumersMu sync.RWMutex
	consumers   map[string]*consumer[T]
	mapFn       func(T) T
	reduceFn    func(T, T) T
	onReady     func(string)
}

/*
SetNotify attaches a reader-aware activity callback. It runs only when that
reader's queue transitions from empty to non-empty, after the mapped value is
retained, so a scheduler can route the owning key without polling every queue.
*/
func (mapReduce *MapReduce[T]) SetNotify(notifyFn func(string)) {
	mapReduce.consumersMu.Lock()
	mapReduce.onReady = notifyFn
	mapReduce.consumersMu.Unlock()
}

func NewMapReduce[T any](
	consumerIDs []string, mapFn func(T) T, reduceFn func(T, T) T,
) *MapReduce[T] {
	mapReduce := &MapReduce[T]{
		consumers: make(map[string]*consumer[T], len(consumerIDs)),
		mapFn: func(item T) T {
			return item
		},
		reduceFn: func(_ T, item T) T {
			return item
		},
	}

	if mapFn != nil {
		mapReduce.mapFn = mapFn
	}

	if reduceFn != nil {
		mapReduce.reduceFn = reduceFn
	}

	for _, consumerID := range consumerIDs {
		mapReduce.consumers[consumerID] = newConsumer[T](0)
	}

	return mapReduce
}

func newConsumer[T any](limit uint64) *consumer[T] {
	return &consumer[T]{
		queue: lf.NewQueue[T](),
		ready: make(chan struct{}, 1),
		limit: limit,
	}
}

/*
Length reports the deepest active consumer queue. It therefore represents the
actual retained backlog rather than a staging queue already copied elsewhere.
*/
func (mapReduce *MapReduce[T]) Length() uint64 {
	mapReduce.consumersMu.RLock()
	defer mapReduce.consumersMu.RUnlock()

	var depth uint64

	for _, consumer := range mapReduce.consumers {
		if queued := consumer.queued.Load(); queued > depth {
			depth = queued
		}
	}

	return depth
}

func (mapReduce *MapReduce[T]) Register(consumerID string) {
	mapReduce.RegisterBounded(consumerID, 0)
}

/*
RegisterBounded creates a consumer with an optional retained-item limit. Once a
bounded consumer reaches its limit it is marked overflowed and accepts no more
items; callers can close that consumer explicitly without silently losing an
unknown suffix of its stream.
*/
func (mapReduce *MapReduce[T]) RegisterBounded(consumerID string, limit uint64) {
	mapReduce.consumersMu.Lock()
	defer mapReduce.consumersMu.Unlock()

	if _, exists := mapReduce.consumers[consumerID]; exists {
		return
	}

	mapReduce.consumers[consumerID] = newConsumer[T](limit)
}

/*
Unregister releases a consumer queue and all values retained exclusively for
it. Producers stop including it in fan-out before this method returns.
*/
func (mapReduce *MapReduce[T]) Unregister(consumerID string) {
	mapReduce.consumersMu.Lock()
	delete(mapReduce.consumers, consumerID)
	mapReduce.consumersMu.Unlock()
}

func (mapReduce *MapReduce[T]) Push(item T) {
	mapReduce.consumersMu.RLock()
	readyConsumers := make([]string, 0)

	for consumerID, consumer := range mapReduce.consumers {
		if consumer.overflowed.Load() {
			continue
		}

		if !consumer.reserve() {
			notify(consumer.ready)
			continue
		}

		consumer.queue.Enqueue(mapReduce.mapFn(item))
		notify(consumer.ready)

		if onReady := mapReduce.onReady; onReady != nil &&
			consumer.scheduled.CompareAndSwap(false, true) {
			readyConsumers = append(readyConsumers, consumerID)
		}
	}

	onReady := mapReduce.onReady
	mapReduce.consumersMu.RUnlock()

	if onReady == nil {
		return
	}

	for _, consumerID := range readyConsumers {
		onReady(consumerID)
	}
}

func (consumer *consumer[T]) reserve() bool {
	if consumer.limit == 0 {
		consumer.queued.Add(1)
		return true
	}

	for {
		queued := consumer.queued.Load()

		if queued >= consumer.limit {
			consumer.overflowed.Store(true)
			return false
		}

		if consumer.queued.CompareAndSwap(queued, queued+1) {
			return true
		}
	}
}

func notify(ready chan struct{}) {
	select {
	case ready <- struct{}{}:
	default:
	}
}

func (mapReduce *MapReduce[T]) Pop(consumerID string) (T, bool) {
	consumer, found := mapReduce.consumer(consumerID)

	if !found {
		var zero T
		return zero, false
	}

	return mapReduce.stage(consumer)
}

/*
WaitPop blocks until a value, cancellation, unregister, or overflow is visible.
The boolean is false for every terminal/no-value result; callers distinguish a
slow-consumer overflow with Overflowed.
*/
func (mapReduce *MapReduce[T]) WaitPop(
	ctx context.Context, consumerID string,
) (T, bool) {
	for {
		consumer, found := mapReduce.consumer(consumerID)

		if !found {
			var zero T
			return zero, false
		}

		if item, ok := mapReduce.stage(consumer); ok {
			return item, true
		}

		if consumer.overflowed.Load() {
			var zero T
			return zero, false
		}

		select {
		case <-ctx.Done():
			var zero T
			return zero, false
		case <-consumer.ready:
		}
	}
}

func (mapReduce *MapReduce[T]) Overflowed(consumerID string) bool {
	consumer, found := mapReduce.consumer(consumerID)
	return found && consumer.overflowed.Load()
}

func (mapReduce *MapReduce[T]) ConsumerLength(consumerID string) uint64 {
	consumer, found := mapReduce.consumer(consumerID)

	if !found {
		return 0
	}

	return consumer.queued.Load()
}

func (mapReduce *MapReduce[T]) Ready(consumerID string) <-chan struct{} {
	consumer, found := mapReduce.consumer(consumerID)

	if !found {
		closed := make(chan struct{})
		close(closed)
		return closed
	}

	return consumer.ready
}

func (mapReduce *MapReduce[T]) consumer(consumerID string) (*consumer[T], bool) {
	mapReduce.consumersMu.RLock()
	consumer, found := mapReduce.consumers[consumerID]
	mapReduce.consumersMu.RUnlock()
	return consumer, found
}

func (mapReduce *MapReduce[T]) Drain(
	consumerID string, predicate func(item T) bool,
) iter.Seq[T] {
	var item T

	return func(yield func(T) bool) {
		defer mapReduce.rearm(consumerID)
		remaining := mapReduce.ConsumerLength(consumerID)

		for remaining > 0 && predicate(item) {
			var ok bool
			item, ok = mapReduce.Pop(consumerID)

			if !ok || !yield(item) {
				return
			}

			remaining--
		}
	}
}

func (mapReduce *MapReduce[T]) stage(consumer *consumer[T]) (T, bool) {
	consumer.stageMu.Lock()
	defer consumer.stageMu.Unlock()

	item, ok := consumer.queue.Dequeue()

	if !ok {
		var zero T
		return zero, false
	}

	consumer.queued.Add(^uint64(0))

	if consumer.hasPrevious {
		item = mapReduce.reduceFn(consumer.previous, item)
	}

	consumer.previous = item
	consumer.hasPrevious = true
	return item, true
}

func (mapReduce *MapReduce[T]) rearm(consumerID string) {
	consumer, found := mapReduce.consumer(consumerID)

	if !found {
		return
	}

	consumer.scheduled.Store(false)
	mapReduce.consumersMu.RLock()
	onReady := mapReduce.onReady
	mapReduce.consumersMu.RUnlock()

	if onReady == nil || consumer.queued.Load() == 0 ||
		!consumer.scheduled.CompareAndSwap(false, true) {
		return
	}

	onReady(consumerID)
}
