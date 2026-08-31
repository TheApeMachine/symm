// Package hindsight implements SYMM's retrospective system-inspection and
// mathematical-validation engine, as specified in hindsight/README.md.
package hindsight

import "strings"

/*
RunID is the identity of one process capture session. It is a stable opaque
string assigned before any capture and independent of any storage backend's
row identity.
*/
type RunID string

/*
Stream identifies one transport stream. It is a stable logical name (spot
public, spot private, level3 child, futures, a captured REST response) rather
than a pointer at a live object, so it survives serialization and storage
migrations unchanged.
*/
type Stream string

/*
StreamEpoch identifies one continuous connection span within a Stream. A
reconnect within the same stream yields a new StreamEpoch, so a frame with
StreamSequence 81 before a disconnect is distinguishable from frame 81 after
reconnect without relying on timestamps.
*/
type StreamEpoch uint64

/*
StreamRef is the operational transport identity of one parsed ingress frame:
which stream it arrived on, which connection span (epoch), and its order within
that span. It is minted and advanced by the websocket transport itself and is
available whether or not Hindsight capture is enabled. Hindsight's
CaptureIdentity records/copies the same fact; live trading reads only this
operational metadata.
*/
type StreamRef struct {
	Stream   Stream      `json:"stream"`
	Epoch    StreamEpoch `json:"epoch"`
	Sequence uint64      `json:"sequence"`
}

/*
CaptureIdentity is the stable identity assigned to one external input BEFORE
parsing. It carries no venue timestamp, no venue sequence, no SQLite row id,
and nothing that depends on the eventual persistence backend or the parsed
event type: only the Run it belongs to, the Stream/epoch it arrived on, and the
monotonic run-local CaptureSequence in which SYMM observed it.

The zero value is deliberately rejected by validation: there is no meaningful
"empty" capture identity, and a parsed event must never be assigned one.
*/
type CaptureIdentity struct {
	Run            RunID           `json:"run"`
	Sequence       CaptureSequence `json:"sequence"`
	Stream         Stream          `json:"stream"`
	StreamEpoch    StreamEpoch     `json:"streamEpoch"`
	StreamSequence uint64          `json:"streamSequence"`
}

/*
Valid reports whether every field pinning the identity to a distinct external
input is populated. A zero Run, an empty Stream, a zero epoch, or a zero
sequence make the identity ambiguous and therefore invalid.
*/
func (identity CaptureIdentity) Valid() bool {
	if strings.TrimSpace(string(identity.Run)) == "" {
		return false
	}

	if strings.TrimSpace(string(identity.Stream)) == "" {
		return false
	}

	if identity.StreamEpoch == 0 {
		return false
	}

	if identity.Sequence == 0 {
		return false
	}

	return true
}

/*
CaptureSequence is the monotonically increasing order in which SYMM observed
external inputs during one Run. It is assigned locally before parsing, is not
exchange time, and is not a wall-clock sort performed after capture. It is the
primary ordering of external observations for causal replay.
*/
type CaptureSequence uint64

/*
EnvelopeRef pins one Workspace Envelope to the exact raw input that produced it
and to its deterministic ordinal within that raw input (§12). A raw frame may
produce zero, one, or many Envelopes; the Ordinal disambiguates them in
deterministic parser order.
*/
type EnvelopeRef struct {
	Origin  CaptureIdentity `json:"origin"`
	Ordinal uint64          `json:"ordinal"`
}

/*
ArtifactID is the stable identity of one durable semantic artifact (§16). Kind
names the artifact family and applies only to diagnostics the identity carries
verbatim, never to how two artifacts are compared.
*/
type ArtifactID struct {
	Kind     string `json:"kind"`
	Identity string `json:"identity"`
}

/*
StateVersion is a monotonic transition marker for one shared resident semantic
state (component + key) advanced concurrently by multiple Workloads (§19). It
records what actually occurred — the exact ordering in which transitions
committed — so Hindsight never guesses cross-ring order after the fact.
*/
type StateVersion struct {
	Component string
	Key       string
	Version   uint64
}

/*
ComponentState identifies one observable shared state cell: the component, the
key/symbol, and the version currently resident.
*/
type ComponentState struct {
	Component string
	Key       string
	Version   uint64
}
