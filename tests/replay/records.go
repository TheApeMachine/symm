package replay

import (
	"time"

	"github.com/theapemachine/symm/types"
)

/*
RawFrame is one external input recorded during capture: its assigned identity,
the receive instant, the endpoint it arrived on, its kind, its payload hash,
and the payload bytes exactly as received.
*/
type RawFrame struct {
	Identity    types.CaptureIdentity `json:"identity"`
	ReceivedAt  time.Time             `json:"receivedAt"`
	Endpoint    string                `json:"endpoint"`
	Kind        string                `json:"kind"`
	PayloadHash string                `json:"payloadHash"`
	Payload     []byte                `json:"payload,omitempty"`
}

/*
EnvelopeManifest records how one raw frame entered Workspace: the Envelope's
identity, the workload that processed it, the domain kind, the symbol, and the
venue time and sequence when supplied.
*/
type EnvelopeManifest struct {
	Envelope      types.EnvelopeRef `json:"envelope"`
	Workload      string            `json:"workload"`
	DomainKind    string            `json:"domainKind"`
	Symbol        string            `json:"symbol"`
	VenueAt       time.Time         `json:"venueAt"`
	VenueSequence string            `json:"venueSequence,omitempty"`
}
