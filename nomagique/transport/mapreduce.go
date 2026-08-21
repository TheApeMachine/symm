package transport

import (
	"iter"
	"sync/atomic"
	"time"

	"github.com/theapemachine/symm/nomagique/types"
	"golang.design/x/lockfree/lf"
)

/*
Consumer is the object that is passed in to the Pop and Drain methods of MapReduce.
This allows us to maintain O(1) lookups, while maintaining concurrency safety without
the need for locks.
*/
type Consumer[T any] struct {
	id       string
	wait     func()
	index    int
	queue    *lf.Queue[T]
	status   atomic.Uint32
	previous atomic.Pointer[T]
	latest   atomic.Pointer[T]
	coalesce bool
}

func NewConsumer[T any](
	id string, wait func(),
) *Consumer[T] {
	return &Consumer[T]{
		id:     id,
		wait:   wait,
		index:  0,
		queue:  lf.NewQueue[T](),
		status: atomic.Uint32{},
	}
}

/*
Coalesce keeps only the newest unpublished item for this consumer. Concurrent
producers overwrite the slot instead of growing an unbounded FIFO, which is
the right bound for audit cursors and other latest-wins readers.
*/
func (consumer *Consumer[T]) Coalesce() *Consumer[T] {
	if consumer != nil {
		consumer.coalesce = true
	}

	return consumer
}

func (consumer *Consumer[T]) depth() uint64 {
	if consumer == nil {
		return 0
	}

	if consumer.coalesce {
		if consumer.latest.Load() != nil {
			return 1
		}

		return 0
	}

	return consumer.queue.Length()
}

/*
MapReduce uses lf.Queue to stage data where multiple consumers need
to read from the same Queue. The registered consumer table is an immutable
atomic snapshot, so producers may iterate it while a stage registers or
unregisters a reader without locking.
*/
type MapReduce[T any] struct {
	consumers atomic.Pointer[[]*Consumer[T]]
	callbacks []func()
	mapFn     func(T) T
	reduceFn  func(T, T) T
	observer  atomic.Pointer[drainObserver]
}

type drainObserver struct {
	begin func(string)
	end   func(string, time.Duration)
}

func NewMapReduce[T any](
	consumers []*Consumer[T], mapFn func(T) T, reduceFn func(T, T) T,
) *MapReduce[T] {
	for i := range consumers {
		consumers[i].index = i
		consumers[i].status.Store(uint32(types.READY))
	}

	mr := &MapReduce[T]{}
	mr.consumers.Store(&consumers)

	// Define a noop function as a default mapper.
	mr.mapFn = func(item T) T {
		return item
	}

	// Override the default map function if a custom one is provided.
	if mapFn != nil {
		mr.mapFn = mapFn
	}

	// Define a noop function as a default for reduce.
	mr.reduceFn = func(a, b T) T {
		return b
	}

	// Override the default reduce function if a custom one is provided.
	if reduceFn != nil {
		mr.reduceFn = reduceFn
	}

	return mr
}

/*
consumersSnapshot loads the immutable consumer table published by the last
registration. Push iterates this snapshot so a concurrent Register can replace
it without making the wire slices racy.
*/
func (mr *MapReduce[T]) consumersSnapshot() []*Consumer[T] {
	if mr == nil {
		return nil
	}

	current := mr.consumers.Load()

	if current == nil {
		return nil
	}

	return *current
}

/*
SetObserver installs the optional clock around each yielded work item. Begin
makes in-flight work visible before a long-running item completes; end receives
the exact time spent in the consumer's processing body, including output writes.
The observer is replaced atomically because diagnostics can be attached after
market ingestion has started.
*/
func (mr *MapReduce[T]) SetObserver(
	begin func(string), end func(string, time.Duration),
) {
	if begin == nil && end == nil {
		mr.observer.Store(nil)
		return
	}

	mr.observer.Store(&drainObserver{begin: begin, end: end})
}

/*
Length returns the total depth of the Queue, optionally including
any staged items in the consumer Queues. If no consumers are provided,
it will return the length of the main Queue only.
*/
func (mr *MapReduce[T]) Length(consumers ...*Consumer[T]) uint64 {
	length := uint64(0)
	listed := consumers

	if len(listed) == 0 {
		listed = mr.consumersSnapshot()
	}

	for _, consumer := range listed {
		length += consumer.depth()
	}

	return length
}

/*
Idle reports whether every registered consumer is ready with no queued work.
*/
func (mr *MapReduce[T]) Idle() bool {
	for _, consumer := range mr.consumersSnapshot() {
		if consumer.status.Load() != uint32(types.READY) {
			return false
		}

		if consumer.depth() != 0 {
			return false
		}

		if consumer.status.Load() != uint32(types.READY) {
			return false
		}
	}

	return true
}

/*
Register a new consumer with a unique ID.
This creates a new Queue for the consumer to read from.
If the consumer ID already exists, it will be ignored.
Registration publishes a fresh immutable consumer table with a compare-and-swap,
so concurrent Push iterations never observe a partially appended slice.
*/
func (mr *MapReduce[T]) Register(consumers ...*Consumer[T]) {
	for {
		current := mr.consumers.Load()

		if current == nil {
			current = new([]*Consumer[T])
			*current = nil

			if !mr.consumers.CompareAndSwap(nil, current) {
				current = mr.consumers.Load()
			}
		}

		existing := *current
		next := make([]*Consumer[T], len(existing), len(existing)+len(consumers))
		copy(next, existing)

		for _, consumer := range consumers {
			consumer.index = len(next)
			consumer.queue = lf.NewQueue[T]()
			consumer.status.Store(uint32(types.READY))
			next = append(next, consumer)
		}

		if mr.consumers.CompareAndSwap(current, &next) {
			return
		}
	}
}

/*
Unregister stops publication to a dynamically registered consumer and releases
the frames still retained by its private queue.
*/
func (mr *MapReduce[T]) Unregister(consumer *Consumer[T]) {
	consumer.status.Store(uint32(types.STOPPED))
	consumer.latest.Store(nil)

	for consumer.queue.Length() > 0 {
		if _, ok := consumer.queue.Dequeue(); !ok {
			return
		}
	}

	consumer.previous.Store(nil)
}

func (mr *MapReduce[T]) Push(item T) {
	mr.publish(item, false)
}

/*
PushLatest replaces any unpublished FIFO items so each consumer holds the
newest value. Event streams that must observe every row should keep using
Push; snapshot columns that only read the current artifact should use this.
*/
func (mr *MapReduce[T]) PushLatest(item T) {
	mr.publish(item, true)
}

func (mr *MapReduce[T]) publish(item T, latest bool) {
	mapped := mr.mapFn(item)

	for _, consumer := range mr.consumersSnapshot() {
		if consumer.status.Load() == uint32(types.STOPPED) {
			continue
		}

		if consumer.coalesce {
			held := new(T)
			*held = mapped
			consumer.latest.Store(held)
			mr.wake(consumer)
			continue
		}

		if latest {
			for consumer.queue.Length() > 0 {
				if _, ok := consumer.queue.Dequeue(); !ok {
					break
				}
			}
		}

		consumer.queue.Enqueue(mapped)
		mr.wake(consumer)
	}
}

func (mr *MapReduce[T]) wake(consumer *Consumer[T]) {
	if mr.Length(consumer) > 0 && consumer.status.CompareAndSwap(
		uint32(types.READY), uint32(types.BUSY),
	) {
		consumer.wait()
	}
}

/*
Pop an item from the Queue.

First we write the item to every consumer's Queue,
then we dequeue from the consumer's Queue.
This ensures that all consumers see the same data,
while maintaining the ergonomics of a single write Queue.
*/
func (mr *MapReduce[T]) Pop(consumer *Consumer[T]) (T, bool) {
	var (
		zero T
	)

	if consumer != nil && consumer.coalesce {
		return mr.stageLatest(consumer)
	}

	if mr.Length(consumer) == 0 {
		return zero, false
	}

	return mr.stage(consumer)
}

/*
Drain reads a cut of the Queue.

It is essentially a series of Pop calls, with a defined cut-off point.
This prevents infinite draining of a Queue that is continuously being written to.

Pass in a function that returns true to continue draining, or false to stop.
It must return true if the item passed in is empty, as the first call to
Drain will always pass in a zero-value item.
*/
func (mr *MapReduce[T]) Drain(
	consumer *Consumer[T], fn func(item T) bool,
) iter.Seq[T] {
	var (
		item T
		cut  uint64
	)

	if fn == nil {
		cut = mr.Length(consumer) + 1

		fn = func(item T) bool {
			cut--
			return cut > 0
		}
	}

	return func(yield func(T) bool) {
		defer func() {
			if !consumer.status.CompareAndSwap(
				uint32(types.BUSY), uint32(types.READY),
			) {
				return
			}

			if mr.Length(consumer) > 0 && consumer.status.CompareAndSwap(
				uint32(types.READY), uint32(types.BUSY),
			) {
				consumer.wait()
			}
		}()

		for fn(item) {
			item, ok := mr.Pop(consumer)

			if !ok {
				return
			}

			observer := mr.observer.Load()

			if observer != nil && observer.begin != nil {
				observer.begin(consumer.id)
			}

			started := time.Time{}

			if observer != nil && observer.end != nil {
				started = time.Now()
			}

			continued := yield(item)

			if observer != nil && observer.end != nil {
				observer.end(consumer.id, time.Since(started))
			}

			if !continued {
				return
			}
		}
	}
}

func (mr *MapReduce[T]) stage(consumer *Consumer[T]) (item T, ok bool) {
	item, ok = consumer.queue.Dequeue()

	if previous := consumer.previous.Load(); previous != nil {
		item = mr.reduceFn(*previous, item)
	}

	consumer.previous.Store(&item)

	return item, ok
}

func (mr *MapReduce[T]) stageLatest(consumer *Consumer[T]) (item T, ok bool) {
	held := consumer.latest.Swap(nil)

	if held == nil {
		return item, false
	}

	item = *held

	if previous := consumer.previous.Load(); previous != nil {
		item = mr.reduceFn(*previous, item)
	}

	consumer.previous.Store(&item)

	return item, true
}
