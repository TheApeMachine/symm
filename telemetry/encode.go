package telemetry

import (
	"sync/atomic"

	flatbuffers "github.com/google/flatbuffers/go"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

var sequence atomic.Uint64

/*
Encode finishes one schema-tagged FlatBuffers dashboard frame. The returned
slice owns its backing array and can be queued without retaining the builder.
*/
func Encode(frame *wire.FrameT) []byte {
	builder := flatbuffers.NewBuilder(1024)
	envelope := (&wire.EnvelopeT{
		Sequence: sequence.Add(1),
		Frame:    frame,
	}).Pack(builder)
	wire.FinishEnvelopeBuffer(builder, envelope)

	encoded := builder.FinishedBytes()
	frameBytes := make([]byte, len(encoded))
	copy(frameBytes, encoded)
	return frameBytes
}
