package audit

import (
	"github.com/theapemachine/symm/telemetry"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

/*
EnvelopeStore is the narrow write boundary EnvelopeRecorder depends on rather
than any concrete storage engine. backtest.Store (SQLite, the codebase's
current audit-event store) already satisfies this signature via WriteEvent; a
future S3-backed store or the jsonl Recorder in this package can too, without
EnvelopeRecorder or its callers changing.
*/
type EnvelopeStore interface {
	WriteEvent(kind string, payload []byte) error
}

/*
EnvelopeRecorder is the recording Observe group's witness (README §12/§17):
it records the same wire-mirrored Envelope state the UI publishes, verbatim,
through whichever EnvelopeStore the caller wires — the store owns overflow
and backpressure policy explicitly (never a hidden queue). It never mutates
the Envelope and its return value is discarded by the Observe HandlerGroup.
*/
type EnvelopeRecorder struct {
	store EnvelopeStore
}

/*
NewEnvelopeRecorder wraps an already-open EnvelopeStore for the Observe graph.
*/
func NewEnvelopeRecorder(store EnvelopeStore) *EnvelopeRecorder {
	return &EnvelopeRecorder{store: store}
}

func (node *EnvelopeRecorder) Step(envelope *types.Envelope) *types.Envelope {
	if node.store == nil || envelope == nil {
		return envelope
	}

	payload := telemetry.Encode(&wire.FrameT{
		Type:  wire.FrameEnvelopeStateFrame,
		Value: &wire.EnvelopeStateFrameT{State: envelope.Encode()},
	})

	_ = node.store.WriteEvent("envelope", payload)

	return envelope
}
