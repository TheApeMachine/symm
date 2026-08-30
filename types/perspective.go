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
	// KindHistoricalAnalogue names the multivariate trajectory-recurrence
	// composition (nearest historical path distance, its percentile, match
	// count, and match location) the historical analogue advisor maintains.
	KindHistoricalAnalogue
	// KindExecutionContext names the joint-facts execution-terrain composition:
	// executed flow presented alongside displayed touch-capacity asymmetry and
	// crossing cost, so "what this flow means" is read against the book it
	// executed into rather than reduced to a single score.
	KindExecutionContext
	// KindDecomposition names the joint-facts event-decomposition composition:
	// arrival frequency alongside economic throughput, so "many-small vs
	// few-large" activity is read from two facts rather than one conflated
	// throughput scalar.
	KindDecomposition
	// KindLiquidityDynamics names the Liquidity signal's own causal historical
	// state: depth/spread baselines, divergences, z-scores and velocities.
	KindLiquidityDynamics
	// KindFlow names the executed-flow / price-response / displayed-structure
	// composition: CVD flow facts beside DepthFlow book structure.
	KindFlow
	// KindOrderDisposition names the Toxicity disposition composition: fill,
	// withdrawal, replenishment, and retreat as distinguishable mechanisms.
	KindOrderDisposition
	// KindArrival names the Hawkes arrival-process state: empirical rates,
	// conditional intensities, background rates and excitation fractions.
	KindArrival
	// KindArrivalQuality names the Hawkes model-behaviour / epistemic context:
	// branching radius, likelihood gains, innovations, expected descendants.
	KindArrivalQuality
	// KindMorphology names the static dimensionless book-shape composition.
	KindMorphology
	// KindMorphologyDynamics names Morphology's causal historical context:
	// baseline, departure z-score and velocity of its shape-change facts.
	KindMorphologyDynamics
	// KindCoordination names the cohort price-path coupling composition:
	// correlation and lead-lag geometry.
	KindCoordination
	// KindCoordinationSupport names the inference/support facts behind a
	// coordination reading: sample counts, p-values, search provenance.
	KindCoordinationSupport
	// KindRelativeState names the cross-sectional price-state composition
	// (breadth, dispersion, directional consensus).
	KindRelativeState
	// KindActivity names the volume-clock activity composition.
	KindActivity
	// KindDerivatives names the leverage/basis/OI/liquidation composition.
	KindDerivatives
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
	case KindHistoricalAnalogue:
		return "historical_analogue"
	case KindExecutionContext:
		return "execution_context"
	case KindDecomposition:
		return "decomposition"
	case KindLiquidityDynamics:
		return "liquidity_dynamics"
	case KindFlow:
		return "flow"
	case KindOrderDisposition:
		return "order_disposition"
	case KindArrival:
		return "arrival"
	case KindArrivalQuality:
		return "arrival_quality"
	case KindMorphology:
		return "morphology"
	case KindMorphologyDynamics:
		return "morphology_dynamics"
	case KindCoordination:
		return "coordination"
	case KindCoordinationSupport:
		return "coordination_support"
	case KindRelativeState:
		return "relative_state"
	case KindActivity:
		return "activity"
	case KindDerivatives:
		return "derivatives"
	default:
		return "unknown"
	}
}

/*
PerspectiveMetricCapacity bounds how many readings one Perspective can carry.
It is a declared structural bound so the payload stays a fixed-size value and
the hot path never allocates a slice per emission. Each Advisor pipeline
declares its own output symbols (see advisor.NewAdvisor), and the Liquidity
pipeline is exactly the widest known composition today: 3 bound metrics, each
contributing 4 named readings (its current value plus its adaptive baseline,
departure z-score, and first difference) — 12 readings, with no spare
capacity. advisor.NewAdvisor panics if a pipeline declares more outputs than
this bound, so a wider future pipeline fails loudly at construction rather
than silently losing readings — raise this constant when that happens.
*/
const PerspectiveMetricCapacity = 12

/*
MetricReading is one named fact a pipeline emitted for one composed metric:
the interned identity of the value (Metric — e.g. a bound metric's raw value,
or one of its derived statistics such as a baseline or z-score) and the value
itself. A consumer determines what a reading means from Metric, never from its
position in the Readings array, so an Advisor's pipeline can emit any number
of named facts per composed metric without Perspective assuming a fixed shape
such as "value plus baseline plus z-score plus velocity" — that shape belongs
to whichever pipeline happens to produce it, not to the generic wire type.

Defined is false when the pipeline has not yet produced this fact (its
required estimator state does not exist yet), so an undefined reading's zero
Value is never mistaken for a real, observed zero.

Maturity, SNR, and SNRDefined carry forward the source Measurement's own
quality facts for the composed metric this reading belongs to — an Advisor
composes already-produced Measurements and must not discard or re-derive the
provenance they already established.

ObservedAt and From are the reading's own temporal provenance: the event-time
instant this fact was last observed and the interval it represents. They are
NOT the Perspective's At — a Perspective composing facts from multiple producer
Workloads (trade CVD, ticker liquidity, Level3 depthflow) carries readings
observed at different instants, and stamping every underlying fact with the
outermost Perspective.At would erase that distinction. A consumer distinguishes
readings by their own ObservedAt/From; an undefined reading has a zero
ObservedAt, never a fabricated copy of the Perspective's clock. From is zero
when the source measurement declared no interval (instantaneous facts).
*/
type MetricReading struct {
	Metric     nmtypes.Symbol
	Value      float64
	Defined    bool
	ObservedAt time.Time
	From       time.Time
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
