package runtime

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/smarty/go-disruptor"
)

/*
ring owns bounded slots and the publication/completion barriers for staged
consumers. Cursor changes and wakeups share one lock. Waiting consumers therefore
park without polling or losing a notification between checking and sleeping.
Processing itself runs outside the lock, concurrently within each stage.
*/
type ring[T any] struct {
	ctx             context.Context
	changed         *sync.Cond
	buffer          []T
	head, completed atomic.Int64
	closed          bool
	workers         sync.WaitGroup
	stopWake        func() bool
}

/* ringReader owns one consumer cursor and the prior stage's dependency barrier. */
type ringReader struct {
	handler      disruptor.Handler
	dependencies []*ringReader
	sequence     int64
	final        bool
}

func newRing[T any](ctx context.Context, capacity uint32) (*ring[T], error) {
	if capacity == 0 || capacity&(capacity-1) != 0 {
		return nil, errors.New("runtime: ring capacity must be a positive power of two")
	}
	ring := &ring[T]{ctx: ctx, changed: sync.NewCond(&sync.Mutex{}), buffer: make([]T, capacity)}
	ring.head.Store(-1)
	ring.completed.Store(-1)
	ring.stopWake = context.AfterFunc(ctx, func() {
		ring.changed.L.Lock()
		ring.changed.Broadcast()
		ring.changed.L.Unlock()
	})
	return ring, nil
}

/* Start wires stage dependencies before starting any consumer. */
func (ring *ring[T]) Start(groups [][]disruptor.Handler) {
	if len(groups) == 0 || len(groups[len(groups)-1]) != 1 {
		panic("runtime: ring requires one final completion consumer")
	}

	var previous []*ringReader

	for index, group := range groups {
		readers := make([]*ringReader, len(group))

		for member, handler := range group {
			reader := &ringReader{handler: handler, dependencies: previous, sequence: -1, final: index == len(groups)-1}
			readers[member] = reader
			ring.workers.Go(func() { ring.consume(reader) })
		}
		previous = readers
	}
}

/* Publish reserves and commits one slot atomically, blocking only at capacity. */
func (ring *ring[T]) Publish(payload T) (int64, bool) {
	ring.changed.L.Lock()
	defer ring.changed.L.Unlock()

	for !ring.closed && ring.ctx.Err() == nil && ring.head.Load()-ring.completed.Load() >= int64(len(ring.buffer)) {
		ring.changed.Wait()
	}

	if ring.closed || ring.ctx.Err() != nil {
		return 0, false
	}
	sequence := ring.head.Load() + 1
	ring.buffer[sequence&int64(len(ring.buffer)-1)] = payload
	ring.head.Store(sequence)
	ring.changed.Broadcast()
	return sequence, true
}

/* consume drains the available prefix after every upstream sibling completes it. */
func (ring *ring[T]) consume(reader *ringReader) {
	for {
		ring.changed.L.Lock()
		lower := reader.sequence + 1
		upper := ring.available(reader)

		for lower > upper {
			if ring.closed && lower > ring.head.Load() {
				ring.changed.L.Unlock()
				return
			}
			ring.changed.Wait()
			upper = ring.available(reader)
		}
		ring.changed.L.Unlock()
		reader.handler.Handle(lower, upper)
		ring.changed.L.Lock()
		reader.sequence = upper

		if reader.final {
			ring.completed.Store(upper)
		}
		ring.changed.Broadcast()
		ring.changed.L.Unlock()
	}
}

/* available reads the minimum upstream cursor while the publication lock is held. */
func (ring *ring[T]) available(reader *ringReader) int64 {
	upper := ring.head.Load()

	for _, dependency := range reader.dependencies {
		upper = min(upper, dependency.sequence)
	}
	return upper
}

/* Await gives a nested Workload completion semantics without a spin loop. */
func (ring *ring[T]) Await(sequence int64) {
	ring.changed.L.Lock()
	defer ring.changed.L.Unlock()

	for ring.completed.Load() < sequence && ring.ctx.Err() == nil {
		ring.changed.Wait()
	}
}

/* Close refuses new publications and joins every consumer after draining its prefix. */
func (ring *ring[T]) Close() {
	ring.changed.L.Lock()
	ring.closed = true
	ring.changed.Broadcast()
	ring.changed.L.Unlock()
	ring.workers.Wait()
	ring.stopWake()
}
