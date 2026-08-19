package types

/*
OpportunityType names an identifiable, actable market opportunity. It is not a
microstructure category: categories answer "what is the book/flow texture right
now?", opportunities answer "which way is this instrument statistically headed,
and in which lifecycle phase?".

A single opportunity is a conjunction of conditioned observations across
several stages of the pipeline. Nothing here is a scalar threshold; each leg
states a condition in terms of the signal's own statistical state, and the
opportunity is scored by how much of the conjunction holds and how strongly it
is contradicted by competing evidence (see logic/graph's epistemic trust).
*/
type OpportunityType string

const (
	OpportunityNone               OpportunityType = ""
	OpportunitySuddenPump         OpportunityType = "sudden_pump"
	OpportunityCoiledCompression  OpportunityType = "coiled_compression"
	OpportunityDailyRiser         OpportunityType = "daily_riser"
	OpportunityInefficientLag     OpportunityType = "inefficient_lag"
	OpportunityAbsorptionReversal OpportunityType = "absorption_reversal"
)

/*
OpportunityLifecycle is the monotone phase of an opportunity's statistical
lifetime. Only Confirming and Accelerating admit new capital; Emergent is too
thin to size, Climax is the exit window, and Exhausting is the failure state.
*/
type OpportunityLifecycle string

const (
	LifecycleEmergent     OpportunityLifecycle = "emergent"
	LifecycleConfirming   OpportunityLifecycle = "confirming"
	LifecycleAccelerating OpportunityLifecycle = "accelerating"
	LifecycleClimax       OpportunityLifecycle = "climax"
	LifecycleExhausting   OpportunityLifecycle = "exhausting"
)

/*
ObservationCondition is one conditional evidence leg of an opportunity. It
reads the same MetricSample a signal already publishes, but interprets the
measured value as a state and states how that state supports or contradicts the
archetype.

MaturityFloor and SeparationFloor are the epistemic floors a leg must clear
before it may vote at all — an immature or ambiguous reading carries no
opinion rather than a hallucinated one.
*/
type ObservationCondition struct {
	Source          SourceType
	Metric          string
	Side            MeasurementSide
	Name            string
	State           string
	Supports        bool
	Contradicts     bool
	MaturityFloor   float64
	SeparationFloor float64
}

/*
OpportunityArchetype is the complete contract for one identifiable opportunity.
The conjunction is scored by how many supporting legs hold and how few
contradicting legs hold; each leg is multiplied by its epistemic trust before
it contributes, so a phantom wall cannot drive a SuddenPump classification.
*/
type OpportunityArchetype struct {
	Type       OpportunityType
	Precursors []CategoryType
	Supports   []ObservationCondition
	Opposes    []ObservationCondition

	/*
		RolloutDynamics names the generative forward model the MCTS rollout
		applies when this archetype is active. It is part of the catalog
		because each opportunity evolves under different physics: a pump is
		an explosive Hawkes decay, a coiled compression is a potential-to-
		kinetic energy release, a daily riser is a low-damping trend.
	*/
	RolloutDynamics string
}
