package store

import (
	"time"

	"github.com/theapemachine/symm/types"
)

const (
	// FrameKind names the raw websocket-frame event domain. Every CaptureSink
	// write from the transport is tagged with it.
	FrameKind = "websocket_frame"
)

/*
Writer is the store's pipeline-facing recorder. It is the single object that
knows how to turn a pipeline fact — a raw transport frame, an envelope snapshot
at a layer boundary, a strategy decision — into a Repository WriteEvent, and it
is what root.go hands to each stage so every recorded layer shares one swappable
engine. It implements both the envelope Node contract and the transport's
CaptureSink contract, delegating persistence to the injected repository.
*/
type Writer struct {
	repository Repository
}

/*
NewWriter builds a Writer over an already-open repository.
*/
func NewWriter(repository Repository) *Writer {
	return &Writer{repository: repository}
}

/*
Capture satisfies kraken.websocket.CaptureSink: it persists one untouched
transport payload with its endpoint and arrival time. The endpoint identities the
stream (public/private/level3/futures); the bytes are stored verbatim.
*/
func (writer *Writer) Capture(endpoint string, payload []byte, receivedAt time.Time) error {
	if writer == nil || writer.repository == nil {
		return nil
	}

	return writer.repository.WriteEvent(
		FrameKind,
		append([]byte(endpoint+"\x00"), payload...),
		receivedAt,
	)
}

/*
Layer is one envelope-stage recorder: it sits at a workload stage boundary (like
system.Diagnostic) and persists the envelope's accumulated state at that exact
point, tagged with the layer's name as the event kind. Because it is placed
after the stage that produced its outputs, the recorded snapshot is what that
layer contributed — measurements after the signal stage, a regime batch after
category, causal output after causal, and so on.
*/
type Layer struct {
	writer *Writer
	name   string
}

/*
NewLayer builds one stage recorder for the given layer name over the shared writer.
*/
func NewLayer(writer *Writer, name string) *Layer {
	return &Layer{writer: writer, name: name}
}

/*
Step records the envelope's current state as this layer's output.
*/
func (layer *Layer) Step(envelope *types.Envelope) *types.Envelope {
	layer.record(envelope)
	return envelope
}

/*
StepBacklog records like Step, ignoring backlog (backpressure belongs to the
runtime diagnostic stamp, not the durable store).
*/
func (layer *Layer) StepBacklog(envelope *types.Envelope, backlog int64) *types.Envelope {
	layer.record(envelope)
	return envelope
}

func (layer *Layer) record(envelope *types.Envelope) {
	if layer == nil || layer.writer == nil || layer.writer.repository == nil || envelope == nil {
		return
	}

	_ = layer.writer.repository.WriteEvent(
		layer.name,
		envelope.EncodeBytes(),
		time.Now().UTC(),
	)
}
