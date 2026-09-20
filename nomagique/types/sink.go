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

// -----------------------------------------------------------------------------
// Float64Sink
// -----------------------------------------------------------------------------

/*
Float64SinkHandler implements Float64Sink_Server using handler callbacks.
*/
type Float64SinkHandler struct {
	onWrite func(context.Context, uint64, float64) error
	onDone  func(context.Context) error
}

func (h *Float64SinkHandler) Write(ctx context.Context, call Float64Sink_write) error {
	eval := call.Args().Evaluation()

	if eval == 0 {
		eval, _ = EvaluationIDFromContext(ctx)
	}

	if h.onWrite != nil {
		return h.onWrite(ctx, eval, call.Args().Value())
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
	onWrite func(context.Context, uint64, float64) error,
	onDone func(context.Context) error,
) Float64Sink {
	return Float64Sink_ServerToClient(&Float64SinkHandler{
		onWrite: onWrite,
		onDone:  onDone,
	})
}

/*
BroadcastFloat64Sink fans out emissions to multiple downstream sinks and preserves evaluation identity.
*/
type BroadcastFloat64Sink struct {
	sinks []Float64Sink
}

func (b *BroadcastFloat64Sink) Write(ctx context.Context, call Float64Sink_write) error {
	eval := call.Args().Evaluation()

	if eval == 0 {
		eval, _ = EvaluationIDFromContext(ctx)
	}

	val := call.Args().Value()

	for _, s := range b.sinks {
		if !capnp.Client(s).IsValid() {
			continue
		}

		if err := s.Write(ctx, func(p Float64Sink_write_Params) error {
			p.SetEvaluation(eval)
			p.SetValue(val)
			return nil
		}); err != nil {
			return err
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

// -----------------------------------------------------------------------------
// Int64Sink
// -----------------------------------------------------------------------------

type Int64SinkHandler struct {
	onWrite func(context.Context, uint64, int64) error
	onDone  func(context.Context) error
}

func (h *Int64SinkHandler) Write(ctx context.Context, call Int64Sink_write) error {
	eval := call.Args().Evaluation()

	if eval == 0 {
		eval, _ = EvaluationIDFromContext(ctx)
	}

	if h.onWrite != nil {
		return h.onWrite(ctx, eval, call.Args().Value())
	}

	return nil
}

func (h *Int64SinkHandler) Done(ctx context.Context, call Int64Sink_done) error {
	if h.onDone != nil {
		return h.onDone(ctx)
	}

	return nil
}

func NewInt64Sink(
	onWrite func(context.Context, uint64, int64) error,
	onDone func(context.Context) error,
) Int64Sink {
	return Int64Sink_ServerToClient(&Int64SinkHandler{
		onWrite: onWrite,
		onDone:  onDone,
	})
}

type BroadcastInt64Sink struct {
	sinks []Int64Sink
}

func (b *BroadcastInt64Sink) Write(ctx context.Context, call Int64Sink_write) error {
	eval := call.Args().Evaluation()

	if eval == 0 {
		eval, _ = EvaluationIDFromContext(ctx)
	}

	val := call.Args().Value()

	for _, s := range b.sinks {
		if !capnp.Client(s).IsValid() {
			continue
		}

		if err := s.Write(ctx, func(p Int64Sink_write_Params) error {
			p.SetEvaluation(eval)
			p.SetValue(val)
			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}

func (b *BroadcastInt64Sink) Done(ctx context.Context, call Int64Sink_done) error {
	for _, s := range b.sinks {
		if capnp.Client(s).IsValid() {
			_, release := s.Done(ctx, nil)
			release()
		}
	}

	return nil
}

func NewBroadcastInt64Sink(sinks ...Int64Sink) Int64Sink {
	return Int64Sink_ServerToClient(&BroadcastInt64Sink{sinks: sinks})
}

// -----------------------------------------------------------------------------
// TextSink
// -----------------------------------------------------------------------------

type TextSinkHandler struct {
	onWrite func(context.Context, uint64, string) error
	onDone  func(context.Context) error
}

func (h *TextSinkHandler) Write(ctx context.Context, call TextSink_write) error {
	eval := call.Args().Evaluation()

	if eval == 0 {
		eval, _ = EvaluationIDFromContext(ctx)
	}

	val, err := call.Args().Value()
	if err != nil {
		return err
	}

	if h.onWrite != nil {
		return h.onWrite(ctx, eval, val)
	}

	return nil
}

func (h *TextSinkHandler) Done(ctx context.Context, call TextSink_done) error {
	if h.onDone != nil {
		return h.onDone(ctx)
	}

	return nil
}

func NewTextSink(
	onWrite func(context.Context, uint64, string) error,
	onDone func(context.Context) error,
) TextSink {
	return TextSink_ServerToClient(&TextSinkHandler{
		onWrite: onWrite,
		onDone:  onDone,
	})
}

type BroadcastTextSink struct {
	sinks []TextSink
}

func (b *BroadcastTextSink) Write(ctx context.Context, call TextSink_write) error {
	eval := call.Args().Evaluation()

	if eval == 0 {
		eval, _ = EvaluationIDFromContext(ctx)
	}

	val, err := call.Args().Value()
	if err != nil {
		return err
	}

	for _, s := range b.sinks {
		if !capnp.Client(s).IsValid() {
			continue
		}

		if err := s.Write(ctx, func(p TextSink_write_Params) error {
			p.SetEvaluation(eval)
			return p.SetValue(val)
		}); err != nil {
			return err
		}
	}

	return nil
}

func (b *BroadcastTextSink) Done(ctx context.Context, call TextSink_done) error {
	for _, s := range b.sinks {
		if capnp.Client(s).IsValid() {
			_, release := s.Done(ctx, nil)
			release()
		}
	}

	return nil
}

func NewBroadcastTextSink(sinks ...TextSink) TextSink {
	return TextSink_ServerToClient(&BroadcastTextSink{sinks: sinks})
}

// -----------------------------------------------------------------------------
// BoolSink
// -----------------------------------------------------------------------------

type BoolSinkHandler struct {
	onWrite func(context.Context, uint64, bool) error
	onDone  func(context.Context) error
}

func (h *BoolSinkHandler) Write(ctx context.Context, call BoolSink_write) error {
	eval := call.Args().Evaluation()

	if eval == 0 {
		eval, _ = EvaluationIDFromContext(ctx)
	}

	if h.onWrite != nil {
		return h.onWrite(ctx, eval, call.Args().Value())
	}

	return nil
}

func (h *BoolSinkHandler) Done(ctx context.Context, call BoolSink_done) error {
	if h.onDone != nil {
		return h.onDone(ctx)
	}

	return nil
}

func NewBoolSink(
	onWrite func(context.Context, uint64, bool) error,
	onDone func(context.Context) error,
) BoolSink {
	return BoolSink_ServerToClient(&BoolSinkHandler{
		onWrite: onWrite,
		onDone:  onDone,
	})
}

type BroadcastBoolSink struct {
	sinks []BoolSink
}

func (b *BroadcastBoolSink) Write(ctx context.Context, call BoolSink_write) error {
	eval := call.Args().Evaluation()

	if eval == 0 {
		eval, _ = EvaluationIDFromContext(ctx)
	}

	val := call.Args().Value()

	for _, s := range b.sinks {
		if !capnp.Client(s).IsValid() {
			continue
		}

		if err := s.Write(ctx, func(p BoolSink_write_Params) error {
			p.SetEvaluation(eval)
			p.SetValue(val)
			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}

func (b *BroadcastBoolSink) Done(ctx context.Context, call BoolSink_done) error {
	for _, s := range b.sinks {
		if capnp.Client(s).IsValid() {
			_, release := s.Done(ctx, nil)
			release()
		}
	}

	return nil
}

func NewBroadcastBoolSink(sinks ...BoolSink) BoolSink {
	return BoolSink_ServerToClient(&BroadcastBoolSink{sinks: sinks})
}

// -----------------------------------------------------------------------------
// DataSink
// -----------------------------------------------------------------------------

type DataSinkHandler struct {
	onWrite func(context.Context, uint64, []byte) error
	onDone  func(context.Context) error
}

func (h *DataSinkHandler) Write(ctx context.Context, call DataSink_write) error {
	eval := call.Args().Evaluation()

	if eval == 0 {
		eval, _ = EvaluationIDFromContext(ctx)
	}

	val, err := call.Args().Value()
	if err != nil {
		return err
	}

	if h.onWrite != nil {
		return h.onWrite(ctx, eval, val)
	}

	return nil
}

func (h *DataSinkHandler) Done(ctx context.Context, call DataSink_done) error {
	if h.onDone != nil {
		return h.onDone(ctx)
	}

	return nil
}

func NewDataSink(
	onWrite func(context.Context, uint64, []byte) error,
	onDone func(context.Context) error,
) DataSink {
	return DataSink_ServerToClient(&DataSinkHandler{
		onWrite: onWrite,
		onDone:  onDone,
	})
}

type BroadcastDataSink struct {
	sinks []DataSink
}

func (b *BroadcastDataSink) Write(ctx context.Context, call DataSink_write) error {
	eval := call.Args().Evaluation()

	if eval == 0 {
		eval, _ = EvaluationIDFromContext(ctx)
	}

	val, err := call.Args().Value()
	if err != nil {
		return err
	}

	for _, s := range b.sinks {
		if !capnp.Client(s).IsValid() {
			continue
		}

		if err := s.Write(ctx, func(p DataSink_write_Params) error {
			p.SetEvaluation(eval)
			return p.SetValue(val)
		}); err != nil {
			return err
		}
	}

	return nil
}

func (b *BroadcastDataSink) Done(ctx context.Context, call DataSink_done) error {
	for _, s := range b.sinks {
		if capnp.Client(s).IsValid() {
			_, release := s.Done(ctx, nil)
			release()
		}
	}

	return nil
}

func NewBroadcastDataSink(sinks ...DataSink) DataSink {
	return DataSink_ServerToClient(&BroadcastDataSink{sinks: sinks})
}
