package transport

import (
	"iter"
	"slices"

	"golang.design/x/lockfree/lf"
)

/*
MapReduce uses lf.Queue to stage data where multiple consumers need
to read from the same Queue.
*/
type MapReduce[T any] struct {
	data           *lf.Queue[T]
	consumerIDs    []string
	consumerQueues []*lf.Queue[T]
	mapFn          func(T) T
	reduceFn       func(T, T) T
	previous       []*T
}

func NewMapReduce[T any](
	consumerIDs []string, mapFn func(T) T, reduceFn func(T, T) T,
) *MapReduce[T] {
	mr := &MapReduce[T]{
		data:        lf.NewQueue[T](),
		consumerIDs: consumerIDs,
	}

	mr.consumerQueues = make([]*lf.Queue[T], len(consumerIDs))

	for id := range consumerIDs {
		mr.consumerQueues[id] = lf.NewQueue[T]()
	}

	mr.previous = make([]*T, len(consumerIDs))

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

func (mr *MapReduce[T]) Length() uint64 {
	return mr.data.Length()
}

/*
Register a new consumer with a unique ID.
This creates a new Queue for the consumer to read from.
If the consumer ID already exists, it will be ignored.
*/
func (mr *MapReduce[T]) Register(consumerID string) {
	if slices.Index(mr.consumerIDs, consumerID) < 0 {
		mr.consumerIDs = append(mr.consumerIDs, consumerID)
		mr.consumerQueues = append(mr.consumerQueues, lf.NewQueue[T]())
		mr.previous = append(mr.previous, nil)
	}
}

func (mr *MapReduce[T]) Push(item T) {
	mr.data.Enqueue(item)
}

/*
Pop an item from the Queue.

First we write the item to every consumer's Queue,
then we dequeue from the consumer's Queue.
This ensures that all consumers see the same data,
while maintaining the ergonomics of a single write Queue.
*/
func (mr *MapReduce[T]) Pop(consumerID string) (T, bool) {
	var (
		zero T
		id   = slices.Index(mr.consumerIDs, consumerID)
	)

	if id < 0 {
		return zero, false
	}

	consumerQueue := mr.consumerQueues[id]

	if consumerQueue.Length() > 0 {
		return mr.stage(id, consumerQueue)
	}

	item, ok := mr.data.Dequeue()

	if !ok {
		return zero, false
	}

	for consumerIndex := range mr.consumerQueues {
		mr.consumerQueues[consumerIndex].Enqueue(mr.mapFn(item))
	}

	return mr.stage(id, consumerQueue)
}

/*
Drain reads a cut of the Queue.

It is essentially a series of Pop calls, with a defined cut-off point.
This prevents infinite draining of a Queue that is continuously being written to.

Pass in a function that returns true to continue draining, or false to stop.
It must return true if the item passed in is empty, as the first call to
Drain will always pass in a zero-value item.
*/
func (mr *MapReduce[T]) Drain(consumerID string, fn func(item T) bool) iter.Seq[T] {
	var (
		item T
	)

	return func(yield func(T) bool) {
		for fn(item) {
			item, ok := mr.Pop(consumerID)

			if !ok {
				return
			}

			if !yield(item) {
				return
			}
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
