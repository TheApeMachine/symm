package hindsight

import "time"

/*
Run is the record of one process capture session (§5, §51). It identifies
enough execution context to make its data interpretable — process start, code
commit, build identity, configuration digest, and schema versions — plus the
explicit capture-integrity state the run currently exposes.
*/
type Run struct {
	ID             RunID             `json:"id"`
	StartedAt      time.Time         `json:"startedAt"`
	CodeCommit     string            `json:"codeCommit"`
	BuildID        string            `json:"buildId"`
	ConfigDigest   string            `json:"configDigest"`
	SchemaVersions map[string]string `json:"schemaVersions,omitempty"`
	Integrity      Integrity         `json:"integrity"`

	// Positions is how many positions the desk actually held during this run,
	// counted from the recorded lifecycle tape. A reader choosing a run to
	// inspect needs to know which runs traded at all: without it, a run that
	// held nothing and a run whose positions are simply hard to find look
	// identical.
	Positions int `json:"positions"`
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
	Envelope      EnvelopeRef `json:"envelope"`
	Workload      string      `json:"workload"`
	DomainKind    string      `json:"domainKind"`
	Symbol        string      `json:"symbol"`
	VenueAt       time.Time   `json:"venueAt"`
	VenueSequence string      `json:"venueSequence,omitempty"`
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
	Envelope              EnvelopeRef   `json:"envelope"`
	Boundary              string        `json:"boundary"`
	Artifact              ArtifactID    `json:"artifact"`
	ArtifactKind          string        `json:"artifactKind,omitempty"`
	ProducedAt            time.Time     `json:"producedAt,omitempty"`
	Component             string        `json:"component,omitempty"`
	ComponentStateVersion uint64        `json:"componentStateVersion,omitempty"`
	ImmediateParents      []EnvelopeRef `json:"immediateParents,omitempty"`
	// SemanticParents names the shared semantic inputs this artifact consumed
	// whose origin envelope was NOT the triggering envelope — e.g. a Flow
	// Perspective on a Level3 event reading CVD last updated by an earlier
	// Trade. Names are stable source/metric identifiers, traversable to their
	// own witnesses by identity.
	SemanticParents []string `json:"semanticParents,omitempty"`
	Payload         []byte   `json:"payload,omitempty"`
}
