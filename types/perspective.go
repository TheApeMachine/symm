package types

import (
	"time"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
PerspectiveKind names the family of descriptive context a Perspective carries.
It is an interned numeric identity, not a rendered string: the hot path compares
and keys on the integer, never on a string built for display. Each concrete
Advisor family owns a distinct kind so their perspectives do not collide on the
same identity.
*/
type PerspectiveKind uint8

const (
	// KindState is the generic composed-metric temporal-context family: the
	// current value, adaptive baseline, z-score, and velocity of each composed
	// measurement metric, relative to its own history.
	KindState PerspectiveKind = iota + 1
	// KindLiquidity names the execution-terrain composition (spread, touch
	// capacity, book imbalance) the liquidity advisor maintains.
	KindLiquidity
)

/*
String returns the display name of a kind. It is for telemetry and UI rendering
only and must never participate in a hot-path comparison or key.
*/
func (kind PerspectiveKind) String() string {
	switch kind {
	case KindState:
		return "state"
	case KindLiquidity:
		return "liquidity"
	default:
		return "unknown"
	}
}

/*
PerspectiveMetricCapacity bounds how many composed-metric readings one
Perspective can carry. It is a declared structural bound so the payload stays a
fixed-size value and the hot path never allocates a slice per emission.
*/
const PerspectiveMetricCapacity = 8

/*
MetricReading is one composed metric's derived temporal context: the current
value plus the adaptive baseline, the z-score (departure from that baseline in
units of its own dispersion), and the first difference, each against the metric's
own history. Metric is the interned identity of the measured quantity this
reading describes, so a consumer can determine what a reading means without
relying on its position in the Readings array. Ready is false until every
derived slot exists, so a not-ready reading's zeros are never mistaken for a
real estimate.

Maturity, SNR, and SNRDefined carry forward the source Measurement's own
quality facts for this metric's most recent observation — an Advisor composes
already-produced Measurements and must not discard or re-derive the
provenance they already established.
*/
type MetricReading struct {
	Metric     nmtypes.Symbol
	Value      float64
	Baseline   float64
	ZScore     float64
	Velocity   float64
	Ready      bool
	Maturity   float64
	SNR        float64
	SNRDefined bool
}

/*
Perspective is the current descriptive output of one Advisor. It is context,
never an instruction: a Perspective describes the present state of composed
measurements relative to their own history and does not choose an action, impose
a gate, or assert an opportunity score.

The envelope is fixed-size apart from the two identity strings; the payload is a
fixed-size array of metric readings, so emitting a Perspective allocates no
per-event slices or maps.
*/
type Perspective struct {
	Symbol string
	/*
		Peer is the counterpart symbol for a relationship-kind perspective and
		is empty for symbol-local and global perspectives.
	*/
	Peer string

	Kind     PerspectiveKind
	At       time.Time
	Sequence uint64

	Readings [PerspectiveMetricCapacity]MetricReading
	Count    int

	/*
		Err carries a pipeline transition failure for this Step. Number only
		commits successful output, so on a genuine failure this Perspective
		describes nothing new: a consumer must check Err before trusting Readings,
		which in that case still reflect the last successfully committed state,
		not this event's contribution.
	*/
	Err error
}

/*
PerspectiveKey is the structural identity of one perspective: the symbol (and
optional peer) plus the kind. It is a comparable value type usable directly as a
map key without allocating or building strings. A PositionID- or global-scoped
perspective is a later extension of this key.
*/
type PerspectiveKey struct {
	Symbol string
	Peer   string
	Kind   PerspectiveKind
}

/*
Key returns the structural identity of the perspective.
*/
func (perspective Perspective) Key() PerspectiveKey {
	return PerspectiveKey{
		Symbol: perspective.Symbol,
		Peer:   perspective.Peer,
		Kind:   perspective.Kind,
	}
}
