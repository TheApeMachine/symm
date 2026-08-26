package runtime

import (
	"context"
	"sync"

	"github.com/theapemachine/errnie"
)

type StreamClass int

const (
	StreamObservational StreamClass = iota
	StreamReliable
)

/*
RingEvent carries a payload and its sequential index within the stream.
*/
type RingEvent struct {
	Sequence int64
	Value    any
}

/*
Ring is a bounded streaming queue. It uses a sync.Mutex to safely support 
Multiple Producers (MPSC) and implements Observational Overwrite correctly.
*/
type Ring struct {
	mu          sync.Mutex
	wakeup      chan struct{}
	buffer      []RingEvent
	capacity    int64
	class       StreamClass
	writeCursor int64
	readCursor  int64
	closed      bool
	dropped     int64
}

func NewRing(capacity int64, class StreamClass) *Ring {
	if capacity <= 0 {
		capacity = 1024
	}

	return &Ring{
		buffer:   make([]RingEvent, capacity),
		capacity: capacity,
		class:    class,
		wakeup:   make(chan struct{}, 1),
	}
}

/*
Enqueue attempts to append a value to the ring.
*/
func (r *Ring) Enqueue(value any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return errnie.Err(errnie.Conflict, "ring: closed", nil)
	}

	occupancy := r.writeCursor - r.readCursor

	if occupancy >= r.capacity {
		if r.class == StreamReliable {
			return errnie.Err(
				errnie.NotAcceptable,
				"ring: reliable stream capacity exceeded",
				nil,
			)
		}

		// Observational overwrite: advance read cursor, dropping the oldest.
		r.readCursor++
		r.dropped++
	}

	index := r.writeCursor % r.capacity
	r.buffer[index] = RingEvent{
		Sequence: r.writeCursor,
		Value:    value,
	}

	r.writeCursor++

	select {
	case r.wakeup <- struct{}{}:
	default:
	}

	return nil
}

/*
Poll fetches the next event if available without blocking.
*/
func (r *Ring) Poll() (RingEvent, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.writeCursor == r.readCursor {
		return RingEvent{}, false
	}

	index := r.readCursor % r.capacity
	event := r.buffer[index]
	r.readCursor++
	return event, true
}

func (r *Ring) WaitNext(ctx context.Context) (RingEvent, bool) {
	for {
		event, ok := r.Poll()
		if ok {
			return event, true
		}

		r.mu.Lock()
		closed := r.closed
		r.mu.Unlock()

		if closed {
			return RingEvent{}, false
		}

		select {
		case <-ctx.Done():
			return RingEvent{}, false
		case <-r.wakeup:
			// Loop around and try to poll again.
		}
	}
}

func (r *Ring) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	if !r.closed {
		r.closed = true
		close(r.wakeup)
	}
}

func (r *Ring) Occupancy() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.writeCursor - r.readCursor
}

func (r *Ring) Capacity() int64 {
	return r.capacity
}

func (r *Ring) Dropped() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dropped
}
