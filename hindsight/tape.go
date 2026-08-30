package hindsight

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"github.com/theapemachine/errnie"
)

/*
Tape is the logical, storage-agnostic capture store (§50). It holds the four
record families — Run, RawFrame, EnvelopeManifest, ArtifactWitness — keyed by
identity rather than by timestamp, row id, or any persistence platform. Every
lookup is exact-provenance: a frame is retrieved by its CaptureIdentity, never
by searching for the record whose timestamp is nearest.

The Tape itself performs no persistence. A Repository-backed sink can mirror the
same records later without changing the identity model, since none of the keys
depend on storage row identity (§50).
*/
type Tape struct {
	run           Run
	frames        map[CaptureIdentity]RawFrame
	framesBySeq   map[CaptureSequence][]CaptureIdentity
	manifests     map[EnvelopeRef]EnvelopeManifest
	origins       map[CaptureIdentity][]EnvelopeRef
	witnesses     map[EnvelopeRef][]ArtifactWitness
	artifactIndex map[ArtifactID]ArtifactWitness
}

/*
NewTape builds an empty Tape for the given Run. A nil Run ID is a validation
error: the tape would have no run boundary (§5).
*/
func NewTape(run Run) (*Tape, error) {
	if run.ID == "" {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "hindsight: tape requires a run identity", nil))
	}

	return &Tape{
		run:           run,
		frames:        make(map[CaptureIdentity]RawFrame),
		framesBySeq:   make(map[CaptureSequence][]CaptureIdentity),
		manifests:     make(map[EnvelopeRef]EnvelopeManifest),
		origins:       make(map[CaptureIdentity][]EnvelopeRef),
		witnesses:     make(map[EnvelopeRef][]ArtifactWitness),
		artifactIndex: make(map[ArtifactID]ArtifactWitness),
	}, nil
}

/*
Run returns the Run record this tape belongs to.
*/
func (tape *Tape) Run() Run {
	if tape == nil {
		return Run{}
	}

	return tape.run
}

/*
WriteFrame records one raw frame under its CaptureIdentity. The frame's payload
hash is recomputed from its payload so a stored hash can never silently disagree
with the bytes (§10, §47). A frame whose identity is invalid or already present
is rejected rather than silently overwriting an earlier observation.
*/
func (tape *Tape) WriteFrame(frame RawFrame) error {
	if tape == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "hindsight: tape required", nil))
	}

	if !frame.Identity.Valid() {
		return errnie.Error(errnie.Err(errnie.Validation, "hindsight: raw frame requires a valid capture identity", nil))
	}

	if _, exists := tape.frames[frame.Identity]; exists {
		return errnie.Error(errnie.Err(errnie.Validation, "hindsight: raw frame already captured", nil))
	}

	frame.PayloadHash = hashPayload(frame.Payload)
	tape.frames[frame.Identity] = frame
	tape.framesBySeq[frame.Identity.Sequence] = append(
		tape.framesBySeq[frame.Identity.Sequence],
		frame.Identity,
	)

	return nil
}

/*
Frame returns the raw frame recorded under exactly the given CaptureIdentity.
It is an exact-provenance lookup: no timestamp search, no nearest-match.
*/
func (tape *Tape) Frame(identity CaptureIdentity) (RawFrame, bool) {
	if tape == nil {
		return RawFrame{}, false
	}

	frame, ok := tape.frames[identity]

	return frame, ok
}

/*
WriteManifest records how one Envelope entered Workspace, keyed by its
EnvelopeRef. The manifest's Envelope identity must carry a valid Origin, and the
Origin is indexed so the raw-frame → envelope fan-out is recoverable exactly.
*/
func (tape *Tape) WriteManifest(manifest EnvelopeManifest) error {
	if tape == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "hindsight: tape required", nil))
	}

	if !manifest.Envelope.Origin.Valid() {
		return errnie.Error(errnie.Err(errnie.Validation, "hindsight: envelope manifest requires a valid origin", nil))
	}

	if _, exists := tape.manifests[manifest.Envelope]; exists {
		return errnie.Error(errnie.Err(errnie.Validation, "hindsight: envelope manifest already recorded", nil))
	}

	tape.manifests[manifest.Envelope] = manifest
	tape.origins[manifest.Envelope.Origin] = append(
		tape.origins[manifest.Envelope.Origin],
		manifest.Envelope,
	)

	return nil
}

/*
ManifestsFor returns the EnvelopeRefs derived from exactly one raw frame,
in deterministic ordinal order (§12). It answers the raw-frame → envelope
fan-out without ever searching by timestamp.
*/
func (tape *Tape) ManifestsFor(origin CaptureIdentity) []EnvelopeRef {
	if tape == nil {
		return nil
	}

	refs := append([]EnvelopeRef(nil), tape.origins[origin]...)

	sort.SliceStable(refs, func(left, right int) bool {
		return refs[left].Ordinal < refs[right].Ordinal
	})

	return refs
}

/*
WriteWitness records one artifact witness at an explicit boundary (§23). The
witnesses are keyed by their EnvelopeRef so the artifact → envelope → raw-frame
chain is traversable, and by ArtifactID for direct artifact lookup.
*/
func (tape *Tape) WriteWitness(witness ArtifactWitness) error {
	if tape == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "hindsight: tape required", nil))
	}

	if !witness.Envelope.Origin.Valid() {
		return errnie.Error(errnie.Err(errnie.Validation, "hindsight: artifact witness requires a valid origin", nil))
	}

	if witness.Artifact.Identity == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "hindsight: artifact witness requires an artifact identity", nil))
	}

	tape.witnesses[witness.Envelope] = append(
		tape.witnesses[witness.Envelope],
		witness,
	)
	tape.artifactIndex[witness.Artifact] = witness

	return nil
}

/*
WitnessesFor returns every artifact witness produced at the boundary of exactly
one Envelope.
*/
func (tape *Tape) WitnessesFor(ref EnvelopeRef) []ArtifactWitness {
	if tape == nil {
		return nil
	}

	return append([]ArtifactWitness(nil), tape.witnesses[ref]...)
}

/*
Artifact returns the witness for exactly one ArtifactID, when recorded.
*/
func (tape *Tape) Artifact(id ArtifactID) (ArtifactWitness, bool) {
	if tape == nil {
		return ArtifactWitness{}, false
	}

	witness, ok := tape.artifactIndex[id]

	return witness, ok
}

/*
Frames returns every recorded raw frame in capture-sequence order (§6, §52), the
order replay feeds the Workspace. Sorting is by CaptureSequence — never by venue
or receive time.
*/
func (tape *Tape) Frames() []RawFrame {
	if tape == nil {
		return nil
	}

	sequences := make([]CaptureSequence, 0, len(tape.framesBySeq))

	for sequence := range tape.framesBySeq {
		sequences = append(sequences, sequence)
	}

	sort.Slice(sequences, func(left, right int) bool {
		return sequences[left] < sequences[right]
	})

	frames := make([]RawFrame, 0, len(tape.frames))

	for _, sequence := range sequences {
		for _, identity := range tape.framesBySeq[sequence] {
			frames = append(frames, tape.frames[identity])
		}
	}

	return frames
}

/*
hashPayload is the canonical payload hash for a raw frame. It is deterministic
and derived only from the bytes, so a stored hash is always recomputable and
never carries provenance of its own.
*/
func hashPayload(payload []byte) string {
	sum := sha256.Sum256(payload)

	return hex.EncodeToString(sum[:])
}
