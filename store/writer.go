package store

import (
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
)

/*
Writer is the store's transport-facing recorder and the single capture authority
for the whole system. It owns the Hindsight Sequencer, so one raw websocket frame
is minted a stable CaptureIdentity, persisted with that identity, and the identity
is returned — in one place — before any pipeline mutation. Every stream shares
the writer's sequencer, so capture identities form one run-local order while the
per-stream epoch/sequence stays distinct.
*/
type Writer struct {
	repository Repository
	sequencer  *hindsight.Sequencer

	mu      sync.Mutex
	streams map[string][]hindsight.Stream
}

/*
NewWriter builds a Writer over an already-open repository and a sequencer.
Constructing with a nil sequencer is a programmer error and is rejected: capture
without identity would silently produce untraceable records.
*/
func NewWriter(repository Repository, sequencer *hindsight.Sequencer) (*Writer, error) {
	if sequencer == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"store: writer requires a capture sequencer",
			nil,
		))
	}

	return &Writer{
		repository: repository,
		sequencer:  sequencer,
		streams:    make(map[string][]hindsight.Stream),
	}, nil
}

/*
Capture satisfies kraken.websocket.CaptureSink: it mints a CaptureIdentity for
one untouched transport payload, persists the frame with that identity, and
returns the identity so the caller can stamp it onto every envelope parsed from
the frame. kind names the frame's channel/method/feed; endpoint names the stream.
The payload is stored as the exact bytes off the wire.
*/
func (writer *Writer) Capture(
	kind, endpoint string,
	payload []byte,
	receivedAt time.Time,
) (hindsight.CaptureIdentity, error) {
	if writer == nil || writer.sequencer == nil {
		return hindsight.CaptureIdentity{}, nil
	}

	// The stream identity comes from the endpoint kind; the writer derives the
	// stable stream name once so minting and persistence agree.
	stream := writer.streamFor(endpoint, kind)

	// Track the stream under its endpoint so a reconnect can bump every epoch
	// the endpoint carries, without the caller enumerating kinds.
	writer.mu.Lock()
	writer.streams[endpoint] = appendUnique(writer.streams[endpoint], stream)
	writer.mu.Unlock()

	identity, err := writer.sequencer.Assign(stream)

	if err != nil {
		return hindsight.CaptureIdentity{}, err
	}

	if writer.repository == nil {
		return identity, nil
	}

	if err := writer.repository.WriteCapture(
		identity,
		endpoint,
		kind,
		payload,
		receivedAt,
	); err != nil {
		return hindsight.CaptureIdentity{}, err
	}

	return identity, nil
}

/*
streamFor names the logical stream a capture belongs to. The endpoint is the
transport source; the kind discriminates within it. This keeps the stream name
stable and independent of the endpoint string's exact URL.
*/
func (writer *Writer) streamFor(endpoint, kind string) hindsight.Stream {
	return hindsight.Stream(endpoint + ":" + kind)
}

/*
Reconnect advances the stream epoch for every stream the writer has seen arrive
on the given endpoint. It is the Hindsight side of a transport reconnect: a new
connection span is a new StreamEpoch, so a frame with StreamSequence N before
the reconnect is distinguishable from frame N after it (§7). The capture sequence
keeps increasing across the reconnect; only the epoch/stream-sequence reset.
*/
func (writer *Writer) Reconnect(endpoint string) {
	if writer == nil || writer.sequencer == nil || endpoint == "" {
		return
	}

	writer.mu.Lock()
	defer writer.mu.Unlock()

	for _, stream := range writer.streams[endpoint] {
		writer.sequencer.Reconnect(stream)
	}
}

/*
appendUnique appends a stream to a slice only if it is not already present,
preserving the seed order. Duplicate tracking of the same stream across frames
is expected (every frame on a stream mints under it), and re-bumping an epoch for
the same stream twice would be wrong, so the set is deduplicated.
*/
func appendUnique(streams []hindsight.Stream, stream hindsight.Stream) []hindsight.Stream {
	for _, existing := range streams {
		if existing == stream {
			return streams
		}
	}

	return append(streams, stream)
}

/*
WriteManifest persists one EnvelopeManifest, delegating to the underlying
repository. It satisfies kraken.websocket.ManifestSink so the ingress layer can
record the raw-frame → envelope fan-out as it parses, without the writer knowing
anything about the parser.
*/
func (writer *Writer) WriteManifest(manifest hindsight.EnvelopeManifest) error {
	if writer == nil || writer.repository == nil {
		return nil
	}

	return writer.repository.WriteManifest(manifest)
}

/*
WriteWitness persists one ArtifactWitness, delegating to the underlying
repository.
*/
func (writer *Writer) WriteWitness(witness hindsight.ArtifactWitness) error {
	if writer == nil || writer.repository == nil {
		return nil
	}

	return writer.repository.WriteWitness(witness)
}
