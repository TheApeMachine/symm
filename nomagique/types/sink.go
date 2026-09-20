package types

import (
	"context"
	"sync/atomic"

	"capnproto.org/go/capnp/v3"
)

type evaluationContextKey struct{}

/*
WithEvaluationID associates an evaluation identity with the context.
Every graph traversal / observation frame carries an evaluation ID.
*/
func WithEvaluationID(ctx context.Context, id uint64) context.Context {
	return context.WithValue(ctx, evaluationContextKey{}, id)
}

/*
EvaluationIDFromContext retrieves the evaluation identity from the context.
*/
func EvaluationIDFromContext(ctx context.Context) (uint64, bool) {
	if ctx == nil {
		return 0, false
	}
	id, ok := ctx.Value(evaluationContextKey{}).(uint64)
	return id, ok
}

var globalEvaluationSequence atomic.Uint64

/*
NextEvaluationContext creates a new context with a monotonically incremented evaluation ID.
*/
func NextEvaluationContext(parent context.Context) (context.Context, uint64) {
	if parent == nil {
		parent = context.Background()
	}
	id := globalEvaluationSequence.Add(1)
	return WithEvaluationID(parent, id), id
}

/*
Float64SinkHandler implements Float64Sink_Server using handler callbacks.
*/
type Float64SinkHandler struct {
	onWrite func(context.Context, float64) error
	onDone  func(context.Context) error
}

func (h *Float64SinkHandler) Write(ctx context.Context, call Float64Sink_write) error {
	if h.onWrite != nil {
		return h.onWrite(ctx, call.Args().Value())
	}
	return nil
}

func (h *Float64SinkHandler) Done(ctx context.Context, call Float64Sink_done) error {
	if h.onDone != nil {
		return h.onDone(ctx)
	}
	return nil
}

/*
NewFloat64Sink creates a local Float64Sink capability backed by the provided write and done handlers.
*/
func NewFloat64Sink(
	onWrite func(context.Context, float64) error,
	onDone func(context.Context) error,
) Float64Sink {
	return Float64Sink_ServerToClient(&Float64SinkHandler{
		onWrite: onWrite,
		onDone:  onDone,
	})
}

/*
BroadcastFloat64Sink fans out emissions to multiple downstream sinks.
*/
type BroadcastFloat64Sink struct {
	sinks []Float64Sink
}

func (b *BroadcastFloat64Sink) Write(ctx context.Context, call Float64Sink_write) error {
	val := call.Args().Value()
	for _, s := range b.sinks {
		if capnp.Client(s).IsValid() {
			if err := s.Write(ctx, func(p Float64Sink_write_Params) error {
				p.SetValue(val)
				return nil
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *BroadcastFloat64Sink) Done(ctx context.Context, call Float64Sink_done) error {
	for _, s := range b.sinks {
		if capnp.Client(s).IsValid() {
			_, release := s.Done(ctx, nil)
			release()
		}
	}
	return nil
}

/*
NewBroadcastFloat64Sink creates a Float64Sink capability that broadcasts each write and done to all provided sinks.
*/
func NewBroadcastFloat64Sink(sinks ...Float64Sink) Float64Sink {
	return Float64Sink_ServerToClient(&BroadcastFloat64Sink{sinks: sinks})
}
