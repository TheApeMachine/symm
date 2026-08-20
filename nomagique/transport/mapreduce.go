package transport

import (
	"iter"
	"sync/atomic"

	"github.com/theapemachine/symm/nomagique/types"
	"golang.design/x/lockfree/lf"
)

/*
Consumer is the object that is passed in to the Pop and Drain methods of MapReduce.
This allows us to maintain O(1) lookups, while maintaining concurrency safety without
the need for locks.
*/
type Consumer[T any] struct {
	wait   func()
	index  int
	queue  *lf.Queue[T]
	status atomic.Uint32
}

func NewConsumer[T any](
	id string, wait func(),
) *Consumer[T] {
	return &Consumer[T]{
		wait:   wait,
		index:  0,
		queue:  lf.NewQueue[T](),
		status: atomic.Uint32{},
	}
}

/*
MapReduce uses lf.Queue to stage data where multiple consumers need
to read from the same Queue.
*/
type MapReduce[T any] struct {
	consumers []*Consumer[T]
	callbacks []func()
	mapFn     func(T) T
	reduceFn  func(T, T) T
	previous  []*T
}

func NewMapReduce[T any](
	consumers []*Consumer[T], mapFn func(T) T, reduceFn func(T, T) T,
) *MapReduce[T] {
	for i := range consumers {
		consumers[i].index = i
		consumers[i].status.Store(uint32(types.READY))
	}

	mr := &MapReduce[T]{
		consumers: consumers,
	}

	mr.previous = make([]*T, len(consumers))

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
Length returns the total depth of the Queue, optionally including
any staged items in the consumer Queues. If no consumers are provided,
it will return the length of the main Queue only.
*/
func (mr *MapReduce[T]) Length(consumers ...*Consumer[T]) uint64 {
	length := uint64(0)

	for _, consumer := range mr.consumers {
		if len(consumers) > 0 {
			length += consumer.queue.Length()
			continue
		}

		length += consumer.queue.Length()
	}

	return length
}

/*
Register a new consumer with a unique ID.
This creates a new Queue for the consumer to read from.
If the consumer ID already exists, it will be ignored.
*/
func (mr *MapReduce[T]) Register(consumers ...*Consumer[T]) {
	for _, consumer := range consumers {
		consumer.index = len(mr.consumers)
		consumer.queue = lf.NewQueue[T]()
		consumer.status.Store(uint32(types.READY))

		mr.consumers = append(mr.consumers, consumer)
		mr.previous = append(mr.previous, nil)
	}
}

func (mr *MapReduce[T]) Push(item T) {
	for i := 0; i < len(mr.consumers); i++ {
		mr.consumers[i].queue.Enqueue(mr.mapFn(item))

		if mr.Length(mr.consumers[i]) > 0 && mr.consumers[i].status.CompareAndSwap(
			uint32(types.READY), uint32(types.BUSY),
		) {
			mr.consumers[i].wait()
		}
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

	if mr.Length(consumer) == 0 {
		return zero, false
	}

	return mr.stage(consumer.index, consumer.queue)
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
		for fn(item) {
			item, ok := mr.Pop(consumer)

			if !ok {
				return
			}

			if !yield(item) {
				return
			}
		}

		consumer.status.Store(uint32(types.READY))

		if mr.Length(consumer) > 0 && consumer.status.CompareAndSwap(
			uint32(types.READY), uint32(types.BUSY),
		) {
			consumer.wait()
		}
	}
}

func (mr *MapReduce[T]) stage(id int, queue *lf.Queue[T]) (item T, ok bool) {
	item, ok = queue.Dequeue()

	if mr.previous[id] != nil {
		item = mr.reduceFn(*mr.previous[id], item)
	}

	mr.previous[id] = &item
	return item, ok
}
