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
is minted a stable CaptureIdentity and accepted into one ordered persistence queue
before the identity is returned for pipeline mutation. A single worker drains raw
frames and manifests in batches. Every stream shares the writer's sequencer, so
capture identities form one run-local order while the per-stream epoch/sequence
stays distinct.
*/
type Writer struct {
	repository Repository
	sequencer  *hindsight.Sequencer

	mu      sync.Mutex
	streams map[string][]hindsight.Stream
	queue   chan writerOperation
	done    chan struct{}
	failed  chan struct{}

	stateMu   sync.RWMutex
	closed    bool
	closeOnce sync.Once
	errorMu   sync.RWMutex
	err       error
	batchSize int
}

/*
NewWriter builds a Writer over an already-open repository and a sequencer.
Constructing with a nil sequencer is a programmer error and is rejected: capture
without identity would silently produce untraceable records.
*/
func NewWriter(
	repository Repository,
	sequencer *hindsight.Sequencer,
	queueDepth, batchSize int,
) (*Writer, error) {
	if sequencer == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"store: writer requires a capture sequencer",
			nil,
		))
	}

	if queueDepth < 1 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"store: writer requires a positive queue depth",
			nil,
		))
	}

	if batchSize < 1 || batchSize > queueDepth {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"store: writer batch size must be within queue depth",
			nil,
		))
	}

	writer := &Writer{
		repository: repository,
		sequencer:  sequencer,
		streams:    make(map[string][]hindsight.Stream),
		queue:      make(chan writerOperation, queueDepth),
		done:       make(chan struct{}),
		failed:     make(chan struct{}),
		batchSize:  batchSize,
	}

	go writer.run()

	return writer, nil
}

/*
Capture satisfies kraken.websocket.CaptureSink: it mints a CaptureIdentity for
one untouched transport payload, accepts the frame into the ordered writer, and
returns the identity so the caller can stamp it onto every envelope parsed from
the frame. kind names the frame's channel/method/feed; endpoint names the stream;
ref is the operational StreamRef the transport minted, which Hindsight records
(copies) rather than supplies. The payload is stored as the exact bytes off the
wire.
*/
func (writer *Writer) Capture(
	kind, endpoint string,
	payload []byte,
	receivedAt time.Time,
	ref hindsight.StreamRef,
) (hindsight.CaptureIdentity, error) {
	if writer == nil || writer.sequencer == nil {
		return hindsight.CaptureIdentity{}, nil
	}

	// The stream identity comes from the endpoint kind; the writer derives the
	// stable stream name once so minting and persistence agree.
	stream := writer.streamFor(endpoint, kind)

	// Track the stream under its endpoint so a transport reconnect can bump
	// every epoch the endpoint carries, without the caller enumerating kinds.
	writer.mu.Lock()
	writer.streams[endpoint] = appendUnique(writer.streams[endpoint], stream)
	writer.mu.Unlock()

	identity, err := writer.sequencer.Assign(stream)

	if err != nil {
		return hindsight.CaptureIdentity{}, err
	}

	// Hindsight observes causality; it does not supply it. The transport owns
	// the epoch/sequence and the writer records the same fact it was handed
	// rather than minting an independent count.
	if ref.Epoch != 0 {
		identity.StreamEpoch = ref.Epoch
		identity.StreamSequence = ref.Sequence

		if ref.Stream != "" {
			identity.Stream = ref.Stream
		}
	}

	if err := writer.enqueue(writerOperation{
		kind:        writerCapture,
		identity:    identity,
		endpoint:    endpoint,
		captureKind: kind,
		payload:     payload,
		at:          receivedAt,
	}); err != nil {
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
WriteManifest accepts one EnvelopeManifest into the same ordered queue as raw
captures. It satisfies kraken.websocket.ManifestSink so the ingress layer can
record raw-frame to envelope fan-out without performing storage IO.
*/
func (writer *Writer) WriteManifest(manifest hindsight.EnvelopeManifest) error {
	if writer == nil {
		return nil
	}

	return writer.enqueue(writerOperation{
		kind:     writerManifest,
		manifest: manifest,
	})
}

/*
WriteLifecycle accepts one broker transition into the shared ordered storage
queue so entry, fill, and close recording never performs IO on a guardian.
*/
func (writer *Writer) WriteLifecycle(
	runID hindsight.RunID,
	event hindsight.LifecycleEvent,
) error {
	if writer == nil {
		return nil
	}

	return writer.enqueue(writerOperation{
		kind:      writerLifecycle,
		runID:     runID,
		lifecycle: event,
	})
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
