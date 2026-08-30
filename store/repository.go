package store

import (
	"time"

	"github.com/theapemachine/symm/hindsight"
)

/*
Repository is the common persistence boundary every store-backed component
writes through. It exposes a narrow, uniform write surface: one run record and
one raw frame record (tagged with its Hindsight CaptureIdentity, origin kind,
and endpoint) per captured external input — so the storage engine behind it
(SQLite today, an S3-compatible object store later) can be swapped without any
pipeline wiring changing. Implementations own their own durability, batching,
and backpressure policy; the writer only reports.
*/
type Repository interface {
	// WriteRun persists one Run record — the process capture session's
	// identity and interpretability metadata (§5).
	WriteRun(run hindsight.Run) error

	// WriteCapture persists one raw transport frame together with the stable
	// CaptureIdentity assigned to it before parsing. endpoint names the source
	// stream (the websocket URL); kind names the frame's channel/method/feed;
	// payload is the exact bytes off the wire, unmodified; at is the arrival
	// instant. The identity is durable, not transient.
	WriteCapture(
		identity hindsight.CaptureIdentity,
		endpoint, kind string,
		payload []byte,
		at time.Time,
	) error

	// WriteFrame persists one raw transport frame without a Hindsight
	// identity. It remains for callers that capture bytes before a sequencer
	// is available; such frames are not traceable with exact provenance.
	WriteFrame(endpoint, kind string, payload []byte, at time.Time) error

	// WriteManifest persists one EnvelopeManifest — how one raw frame entered
	// Workspace — keyed by its EnvelopeRef so raw capture and semantic ingress
	// are joinable by identity.
	WriteManifest(manifest hindsight.EnvelopeManifest) error

	// WriteWitness persists one ArtifactWitness — the semantic artifact the
	// running binary actually produced at a Workspace boundary, with its exact
	// parent and resident-state provenance.
	WriteWitness(witness hindsight.ArtifactWitness) error

	// MarkGapped persists a concrete capture defect and flips the run's
	// integrity to GAPPED (the highest severity wins if already CORRUPT).
	// detail names the exact failure so an inspector sees why the run is not
	// complete.
	MarkGapped(runID hindsight.RunID, sequence hindsight.CaptureSequence, encoding string, detail string) error

	// MarkCorrupt persists a concrete integrity/provenance defect and flips
	// the run to CORRUPT. Corruption is the strongest verdict and always wins
	// over COMPLETE or GAPPED.
	MarkCorrupt(runID hindsight.RunID, sequence hindsight.CaptureSequence, encoding string, detail string) error

	// Close releases the store's underlying resources.
	Close() error
}
