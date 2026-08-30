package hindsight

import "time"

/*
Run is the record of one process capture session (§5, §51). It identifies
enough execution context to make its data interpretable — process start, code
commit, build identity, configuration digest, and schema versions — plus the
explicit capture-integrity state the run currently exposes.
*/
type Run struct {
	ID             RunID
	StartedAt      time.Time
	CodeCommit     string
	BuildID        string
	ConfigDigest   string
	SchemaVersions map[string]string
	Integrity      Integrity
}

/*
RawFrame is one irreducible external input (§10, §51): its assigned identity,
the receive instant, the endpoint it arrived on, its kind, its payload hash,
and the payload bytes exactly as received. The payload must never be a
reconstructed or normalized equivalent.
*/
type RawFrame struct {
	Identity    CaptureIdentity
	ReceivedAt  time.Time
	Endpoint    string
	Kind        string
	PayloadHash string
	Payload     []byte
}

/*
EnvelopeManifest records exactly how one raw frame entered Workspace (§13,
§51): the Envelope's identity, the workload that processed it, the domain kind,
the symbol/key where applicable, and the venue time and sequence where the
protocol supplied them.
*/
type EnvelopeManifest struct {
	Envelope      EnvelopeRef
	Workload      string
	DomainKind    string
	Symbol        string
	VenueAt       time.Time
	VenueSequence string
}

/*
ArtifactWitness is evidence of what the live system actually produced at one
explicit Workspace boundary (§22, §23, §24.1, §51): the artifact it produced,
the boundary it was observed at, the component and its state version, the
immediate parents that causally produced it, and the artifact payload.

It records what the running binary actually produced. It never recomputes that
output.
*/
type ArtifactWitness struct {
	Envelope              EnvelopeRef
	Boundary              string
	Artifact              ArtifactID
	ArtifactKind          string
	ProducedAt            time.Time
	Component             string
	ComponentStateVersion uint64
	ImmediateParents      []EnvelopeRef
	Payload               []byte
}
