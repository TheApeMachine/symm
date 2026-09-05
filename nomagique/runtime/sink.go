package runtime

import (
	"sync/atomic"
)

/*
Sink is a Node that hands the ring's values to a consumer goroutine over a
channel, instead of running the consumer's work on the ring's own goroutine.

A stage that publishes somewhere — a websocket, a viewer transport, a file —
does not belong on a ring handler: the ring advances at ingress rate and every
consumer mounted as a Node pays its own cost at that rate, whether or not
anything is listening. Declaring a Sink in the stage instead keeps the ring's
obligation to one channel send, and lets the consumer own its work, its rate,
and its state on a goroutine of its own — where single ownership means it needs
no locks to hold that state.

The send never blocks. A full channel drops the value and counts it: a consumer
of a live observation stream is watching, not accounting, and the ring must
never wait on one. Dropped() reports what a consumer missed, so "the viewer is
behind" is a number rather than a silence.
*/
type Sink[T any] struct {
	out     chan T
	dropped atomic.Int64
}

/*
NewSink constructs a Sink buffering up to capacity values. Capacity is how far
a consumer may fall behind before its values start being dropped, so it is set
by how bursty the consumer is, never by the ring's rate.
*/
func NewSink[T any](capacity int) *Sink[T] {
	if capacity < 1 {
		capacity = 1
	}

	return &Sink[T]{out: make(chan T, capacity)}
}

/*
Step offers one value to the consumer and returns it unchanged. It is the whole
of this node's ring-side cost.
*/
func (sink *Sink[T]) Step(value T) T {
	select {
	case sink.out <- value:
	default:
		sink.dropped.Add(1)
	}

	return value
}

/* Out is the consumer's side of the channel. */
func (sink *Sink[T]) Out() <-chan T { return sink.out }

/* Dropped counts the values offered while the consumer was too far behind. */
func (sink *Sink[T]) Dropped() int64 { return sink.dropped.Load() }
