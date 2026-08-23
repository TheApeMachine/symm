package types

import "sync/atomic"

/*
Stream owns one reducer state and is the allocation-free path for ordered,
single-writer event processing.
*/
type Stream struct {
	primitive Primitive
	state     Frame
	output    Frame
	err       error
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
Step evaluates and commits one input. Failed candidates leave state untouched.
*/
func (stream *Stream) Step(input Frame) (Frame, error) {
	if stream == nil || stream.primitive == nil {
		return Frame{}, primitiveError("stream primitive is nil")
	}

	nextState, output, err := Step(stream.primitive, stream.state, input)

	if err != nil {
		stream.err = err

		return stream.output, err
	}

	stream.state = nextState
	stream.output = output
	stream.err = nil

	return output, nil
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
Output returns the last successful output snapshot.
*/
func (stream *Stream) Output() Frame {
	if stream == nil {
		return Frame{}
	}

	return stream.output
}

/*
Error returns the last transition failure.
*/
func (stream *Stream) Error() error {
	if stream == nil {
		return primitiveError("stream is nil")
	}

	return stream.err
}

/*
Reset replaces committed state and clears the last output and error.
*/
func (stream *Stream) Reset(initial Frame) {
	if stream == nil {
		return
	}

	stream.state = initial
	stream.output = Frame{}
	stream.err = nil
}

/*
AtomicStream provides lock-free CAS commits and projections for read-heavy
workloads with concurrent writers. Immutable pointer publication necessarily
allocates a snapshot per attempted transition; use Stream behind an SPSC ring
when zero-allocation ordered processing is required.
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
func (stream *AtomicStream) Step(input Frame) (Frame, error) {
	if stream == nil || stream.primitive == nil {
		return Frame{}, primitiveError("atomic stream primitive is nil")
	}

	for {
		current := stream.state.Load()
		nextState, output, err := Step(stream.primitive, *current, input)

		if err != nil {
			return Frame{}, err
		}

		candidate := new(Frame)
		*candidate = nextState

		if stream.state.CompareAndSwap(current, candidate) {
			return output, nil
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
