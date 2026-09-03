package types

import "time"

/*
OpportunityArchetype names the kind of asymmetric situation an opportunity
hypothesis is positioning for. It is a distinct enumeration from CategoryType:
Category describes the present market state, while an archetype names the
eventual transition that state may be forming toward. The mapping between the
two vocabularies lives in the opportunity synthesizer's family table, not here.
*/
type OpportunityArchetype string

const (
	// ArchetypeVerticalIgnition positions for a vertical breakout that has not
	// yet become visible: the coiled/hidden/thinning precursor state that
	// historically precedes the VerticalIgnition category.
	ArchetypeVerticalIgnition OpportunityArchetype = "vertical_ignition"

	// ArchetypeVolumeSurgePrecursor positions for explosive vertical moves
	// detected by sharp precursor volume acceleration (e.g. 3100x volume surge)
	// before visible price ignition.
	ArchetypeVolumeSurgePrecursor OpportunityArchetype = "volume_surge_precursor"
)

/*
OpportunityPhase is the lifecycle stage of one opportunity hypothesis.

	Dormant     → no precursor evidence
	Forming     → one precursor system active
	Armed       → multiple precursor systems agree
	Ignition    → the visible transition (confirmation) has begun
	Invalidated → the precursor state dissolved without igniting

Management and exit are not phases here: once a candidate ignites, the position
lifecycle belongs to the desk and Passage, not to the opportunity tracker.
*/
type OpportunityPhase string

const (
	PhaseDormant     OpportunityPhase = "dormant"
	PhaseForming     OpportunityPhase = "forming"
	PhaseArmed       OpportunityPhase = "armed"
	PhaseIgnition    OpportunityPhase = "ignition"
	PhaseInvalidated OpportunityPhase = "invalidated"
)

/*
OpportunityDirection is the signed exposure an opportunity family implies, when
the family is directional at all. Neutral is for families that name a regime
rather than a side.
*/
type OpportunityDirection int8

const (
	DirectionNeutral OpportunityDirection = 0
	DirectionLong    OpportunityDirection = 1
	DirectionShort   OpportunityDirection = -1
)

/*
OpportunityProvenance is a bitmask of the subsystems whose outputs contributed
evidence to a candidate. It is an interned identity rather than a string list:
the hot path never allocates provenance slices.
*/
type OpportunityProvenance uint8

const (
	ProvenanceCategory  OpportunityProvenance = 1 << iota // 1
	ProvenanceCognition                                   // 2
	ProvenanceManifold                                    // 4
	ProvenanceGraph                                       // 8
	ProvenanceCausal                                      // 16
	ProvenanceResonance                                   // 32
)

/*
Excursion is a fixed three-point summary of a projected excursion distribution.
It is deliberately not a slice: three ordered quantile marks are enough to
express conservative/modal/favorable outcomes without per-candidate allocation.
*/
type Excursion struct {
	Low  float64 `json:"low"`
	Mid  float64 `json:"mid"`
	High float64 `json:"high"`
}

/*
OpportunityEconomics carries the economically meaningful estimates of one
opportunity hypothesis. Calibrated is false until these are actually estimated
from Passage/Resonance history: zero values are placeholders, never fabricated
confidence. Not every archetype populates every field.
*/
type OpportunityEconomics struct {
	Calibrated bool `json:"calibrated"`

	// TransitionProbability is P(the relevant transition occurs), when
	// calibrated. It is a probability, not an opportunity score.
	TransitionProbability float64 `json:"transitionProbability,omitempty"`

	// ProfitFirst is P(the profit boundary precedes invalidation).
	ProfitFirst float64 `json:"profitFirst,omitempty"`

	// FavorableExcursion and AdverseExcursion are the projected favorable and
	// adverse price excursions around a candidate entry.
	FavorableExcursion Excursion `json:"favorableExcursion,omitempty"`
	AdverseExcursion   Excursion `json:"adverseExcursion,omitempty"`

	// Resolution is the expected time until the hypothesis resolves.
	Resolution time.Duration `json:"resolutionNs,omitempty"`

	// Uncertainty is the model's own spread over the estimates above.
	Uncertainty float64 `json:"uncertainty,omitempty"`
}

/*
OpportunityCandidate is the first-class live opportunity hypothesis connecting
semantic evidence to the action layer. It is a typed hypothesis, never a score:
there is no scalar "opportunity strength". Every field is a fixed, interned or
numeric identity so the hot path allocates nothing per candidate.
*/
type OpportunityCandidate struct {
	Symbol     string                `json:"symbol"`
	Archetype  OpportunityArchetype  `json:"archetype"`
	Phase      OpportunityPhase      `json:"phase"`
	Direction  OpportunityDirection  `json:"direction"`
	FirstSeen  time.Time             `json:"firstSeen"`
	Updated    time.Time             `json:"updated"`
	Sequence   uint64                `json:"sequence"`
	Provenance OpportunityProvenance `json:"provenance"`
	Maturity   float64               `json:"maturity"`

	// Economics is nil until downstream stages estimate the economically
	// meaningful dimensions; the synthesizer itself only tracks state.
	Economics *OpportunityEconomics `json:"economics,omitempty"`
}
