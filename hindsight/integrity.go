package hindsight

/*
Integrity is the explicit capture-integrity state a Run exposes (§47). It must
be one of three states: complete, gapped, or corrupt. Hindsight certainty fails
closed on anything but Complete.
*/
type Integrity uint8

const (
	/*
		IntegrityComplete means every captured external input is accounted for
		and referenceable, and no provenance relationship is broken.
	*/
	IntegrityComplete Integrity = iota

	/*
		IntegrityGapped means the run is missing at least one observation — a
		missing CaptureSequence, a persistence failure, a missing payload, a
		broken stream epoch — so inspection across the affected interval cannot
		claim to be complete.
	*/
	IntegrityGapped

	/*
		IntegrityCorrupt means a captured record is present but internally
		broken: a payload hash mismatch, a malformed provenance relationship,
		or an Envelope referencing an absent RawFrame.
	*/
	IntegrityCorrupt
)

func (integrity Integrity) String() string {
	switch integrity {
	case IntegrityComplete:
		return "COMPLETE"
	case IntegrityGapped:
		return "GAPPED"
	case IntegrityCorrupt:
		return "CORRUPT"
	default:
		return "UNKNOWN"
	}
}

/*
MarshalJSON serializes the integrity state as its stable name, so a persisted or
wire representation carries "COMPLETE"/"GAPPED"/"CORRUPT" rather than a raw
enum ordinal that would silently break when the enum reorders.
*/
func (integrity Integrity) MarshalJSON() ([]byte, error) {
	return []byte(`"` + integrity.String() + `"`), nil
}

/*
UnmarshalJSON recovers an integrity state from its stable name, failing closed
to CORRUPT on an unknown value rather than silently reading COMPLETE.
*/
func (integrity *Integrity) UnmarshalJSON(data []byte) error {
	switch string(data) {
	case `"COMPLETE"`:
		*integrity = IntegrityComplete
	case `"GAPPED"`:
		*integrity = IntegrityGapped
	case `"CORRUPT"`:
		*integrity = IntegrityCorrupt
	default:
		*integrity = IntegrityCorrupt
	}

	return nil
}

/*
GapEncoding enumerates the concrete conditions the specification (§47) names as
gap or corruption causes. They are typed so a detector records exactly what it
found rather than collapsing every failure into a bare string.
*/
type GapEncoding uint8

const (
	/*
		GapMissingSequence: a CaptureSequence in the run has no captured input.
	*/
	GapMissingSequence GapEncoding = iota

	/*
		GapPersistenceFailure: a raw input was observed but its persistence
		did not commit.
	*/
	GapPersistenceFailure

	/*
		GapMissingPayload: a raw frame record carries no payload.
	*/
	GapMissingPayload

	/*
		GapHashMismatch: a raw frame's payload does not match its recorded
		PayloadHash.
	*/
	GapHashMismatch

	/*
		GapDanglingEnvelope: an Envelope references a CaptureIdentity with no
		corresponding RawFrame.
	*/
	GapDanglingEnvelope

	/*
		GapMissingOrdinal: a produced Envelope carries no deterministic
		ordinal within its raw frame.
	*/
	GapMissingOrdinal

	/*
		GapBrokenEpoch: a stream epoch reference is missing or inconsistent.
	*/
	GapBrokenEpoch

	/*
		GapDanglingWitness: an artifact witness references an Envelope that
		does not exist.
	*/
	GapDanglingWitness

	/*
		GapMalformedProvenance: a derived artifact's provenance relationship
		does not resolve to its declared inputs.
	*/
	GapMalformedProvenance
)

/*
Gap is one detected integrity defect: its class and, where known, the capture
sequence it affects. A zero CaptureSequence means the defect is not pinned to a
single captured input (e.g. a malformed provenance relationship).
*/
type Gap struct {
	Encoding GapEncoding
	Sequence CaptureSequence
}
