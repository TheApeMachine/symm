// Package hindsight implements SYMM's retrospective system-inspection and
// mathematical-validation engine, as specified in hindsight/README.md.
package hindsight

import "github.com/theapemachine/symm/types"

/*
Capture identity is the record's own vocabulary and belongs with the envelope
it identifies, so it lives in types where both the running system and this
package can speak it.

These are aliases, not copies: hindsight.RunID and types.RunID are one type. The
names stay readable at every existing call site while the dependency runs one
way — hindsight reads what the system stored, rather than the system depending
on the inspector to name its own coordinates.
*/
type (
	RunID           = types.RunID
	Stream          = types.Stream
	StreamEpoch     = types.StreamEpoch
	StreamRef       = types.StreamRef
	CaptureIdentity = types.CaptureIdentity
	CaptureSequence = types.CaptureSequence
	EnvelopeRef     = types.EnvelopeRef
	ArtifactID      = types.ArtifactID
	StateVersion    = types.StateVersion
	ComponentState  = types.ComponentState
)
