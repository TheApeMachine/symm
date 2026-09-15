package runtime

import (
	"context"
	"io"

	"github.com/theapemachine/symm/nomagique/data"
	"golang.design/x/lockfree/wf"
)

/*
Tee unites an off-ramp pusher with a consumer receiver.
*/
type Tee[T any] interface {
	Push(*data.Measurement[float64])
	Next() T
	io.Closer
}

type memoryTee[T any] struct {
	*System
	ring *wf.RingBuffer[T]
}

/*
NewTee creates a new wait-free Tee for type T.
*/
func NewTee[T any](label string, capacity int) Tee[T] {
	tee := &memoryTee[T]{
		ring: wf.NewRingBuffer[T](capacity),
	}

	tee.System = NewSystem(context.Background(), label, tee)
	tee.Transition(READY)

	return tee
}

/*
Push enqueues a measurement onto the wait-free ring buffer.
*/
func (tee *memoryTee[T]) Push(item *data.Measurement[float64]) {
	if val, ok := any(item).(T); ok {
		if !tee.ring.Put(val) {
			tee.Transition(ERROR)
		}
	}
}

/*
Next dequeues the next item from the ring buffer, or returns the zero value if empty.
*/
func (tee *memoryTee[T]) Next() T {
	item, ok := tee.ring.Get()

	if !ok {
		var zero T
		return zero
	}

	return item
}
