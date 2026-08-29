package types

import "sync/atomic"

/*
Stream owns one reducer state and is the allocation-free path for ordered,
single-writer event processing. The frame it holds is both committed state and
the last output; a failed transition leaves it unchanged and records Err.
*/
type Stream struct {
	primitive Primitive
	state     Frame
}

/*
NewStream creates a single-writer stream with an initial state snapshot.
*/
func NewStream(primitive Primitive, initial Frame) *Stream {
	return &Stream{
		primitive: primitive,
		state:     initial,
	}
}

/*
Step evaluates and commits one input. A failed transition sets the returned
frame's Err and leaves committed state untouched.
*/
func (stream *Stream) Step(input Frame) Frame {
	if stream == nil || stream.primitive == nil {
		input.Err = PrimitiveError("stream primitive is nil")

		return input
	}

	merged := stream.state
	merged.Merge(input)
	Step(stream.primitive, &merged)

	if merged.Err == nil {
		stream.state = merged
	}

	return merged
}

/*
Project returns the last committed state snapshot.
*/
func (stream *Stream) Project() Frame {
	if stream == nil {
		return Frame{}
	}

	return stream.state
}

/*
Output returns the last successful output snapshot, which is the committed state.
*/
func (stream *Stream) Output() Frame {
	if stream == nil {
		return Frame{}
	}

	return stream.state
}

/*
Error returns the last transition failure.
*/
func (stream *Stream) Error() error {
	if stream == nil {
		return PrimitiveError("stream is nil")
	}

	return stream.state.Err
}

/*
Reset replaces committed state.
*/
func (stream *Stream) Reset(initial Frame) {
	if stream == nil {
		return
	}

	stream.state = initial
}

/*
AtomicStream provides lock-free CAS commits and projections for read-heavy
workloads with concurrent writers.
*/
type AtomicStream struct {
	state     atomic.Pointer[Frame]
	primitive Primitive
}

/*
NewAtomicStream creates an atomically projected reducer state.
*/
func NewAtomicStream(primitive Primitive, initial Frame) *AtomicStream {
	stream := &AtomicStream{primitive: primitive}
	initialSnapshot := new(Frame)
	*initialSnapshot = initial
	stream.state.Store(initialSnapshot)

	return stream
}

/*
Step applies one lock-free compare-and-swap transition.
*/
func (stream *AtomicStream) Step(input Frame) Frame {
	if stream == nil || stream.primitive == nil {
		input.Err = PrimitiveError("atomic stream primitive is nil")

		return input
	}

	for {
		current := stream.state.Load()
		merged := *current
		merged.Merge(input)
		Step(stream.primitive, &merged)

		if merged.Err != nil {
			return merged
		}

		candidate := new(Frame)
		*candidate = merged

		if stream.state.CompareAndSwap(current, candidate) {
			return merged
		}
	}
}

/*
Project returns an immutable copy of the currently published state.
*/
func (stream *AtomicStream) Project() Frame {
	if stream == nil {
		return Frame{}
	}

	current := stream.state.Load()

	if current == nil {
		return Frame{}
	}

	return *current
}
