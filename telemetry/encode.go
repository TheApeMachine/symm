package telemetry

import (
	"fmt"
	"sync"
	"sync/atomic"

	flatbuffers "github.com/google/flatbuffers/go"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

var sequence atomic.Uint64
var builders = sync.Pool{
	New: func() any { return flatbuffers.NewBuilder(16384) },
}
var entryOffsets = sync.Pool{
	New: func() any { return make([]flatbuffers.UOffsetT, 0) },
}

const BatchIdentifier = "SYMB"

/*
BatchBuffer owns a pooled FlatBuffers builder until Release. Its bytes remain
valid for the duration of a synchronous websocket write and are never copied
into an intermediate transport envelope.
*/
type BatchBuffer struct {
	Bytes   []byte
	builder *flatbuffers.Builder
}

/*
Release returns the batch storage to the encoder pool after the socket write.
*/
func (buffer *BatchBuffer) Release() {
	if buffer == nil || buffer.builder == nil {
		return
	}

	buffer.builder.Reset()
	builders.Put(buffer.builder)
	buffer.Bytes = nil
	buffer.builder = nil
}

/*
Encode finishes one schema-tagged FlatBuffers dashboard frame. The returned
slice owns its backing array and can be queued without retaining the builder.
*/
func Encode(frame *wire.FrameT) []byte {
	builder := builders.Get().(*flatbuffers.Builder)
	defer func() {
		builder.Reset()
		builders.Put(builder)
	}()
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

/*
EncodeBatch makes one FlatBuffers websocket message directly from schema
objects. The returned bytes alias pooled storage and must be released after the
synchronous write completes.
*/
func EncodeBatch(frames []*wire.FrameT) *BatchBuffer {
	builder := builders.Get().(*flatbuffers.Builder)
	offsets := entryOffsets.Get().([]flatbuffers.UOffsetT)

	if cap(offsets) < len(frames) {
		offsets = make([]flatbuffers.UOffsetT, len(frames))
	} else {
		offsets = offsets[:len(frames)]
	}

	for index, frame := range frames {
		frameOffset := frame.Pack(builder)
		wire.FrameEntryStart(builder)
		wire.FrameEntryAddFrameType(builder, frame.Type)
		wire.FrameEntryAddFrame(builder, frameOffset)
		offsets[index] = wire.FrameEntryEnd(builder)
	}

	wire.BatchStartFramesVector(builder, len(offsets))

	for index := len(offsets) - 1; index >= 0; index-- {
		builder.PrependUOffsetT(offsets[index])
	}

	framesOffset := builder.EndVector(len(offsets))
	wire.BatchStart(builder)
	wire.BatchAddSequence(builder, sequence.Add(1))
	wire.BatchAddFrames(builder, framesOffset)
	batch := wire.BatchEnd(builder)
	builder.FinishWithFileIdentifier(batch, []byte(BatchIdentifier))
	entryOffsets.Put(offsets[:0])

	return &BatchBuffer{Bytes: builder.FinishedBytes(), builder: builder}
}

/*
Decode opens one internally produced telemetry frame after checking its schema
identifier. It is used by tests and non-browser consumers that need the object
API rather than FlatBuffers table access.
*/
func Decode(encoded []byte) (*wire.EnvelopeT, error) {
	if !wire.EnvelopeBufferHasIdentifier(encoded) {
		return nil, fmt.Errorf("telemetry: frame has no SYMM schema identifier")
	}

	return wire.GetRootAsEnvelope(encoded, 0).UnPack(), nil
}
