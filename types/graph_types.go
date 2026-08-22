package types

/*
RelationType describes directional edge relationships between market nodes.
*/
type RelationType string

const (
	RelationSupports         RelationType = "supports"
	RelationContradicts      RelationType = "contradicts"
	RelationConditions       RelationType = "conditions"
	RelationLeads            RelationType = "leads"
	RelationLags             RelationType = "lags"
	RelationRedundantWith    RelationType = "redundant_with"
	RelationIndependentOf    RelationType = "independent_of"
	RelationStaleRelativeTo  RelationType = "stale_relative_to"
	RelationIncomparableWith RelationType = "incomparable_with"
)

/*
Kind describes the type of node in the knowledge graph.
*/
type Kind string

const (
	KindMeasurement Kind = "measurement"
	KindCategory    Kind = "category"
	KindManifold    Kind = "manifold"
	KindResonance   Kind = "resonance"
	KindCausal      Kind = "causal"
	KindCognition   Kind = "cognition"
	KindPrediction  Kind = "prediction"
	KindHypothesis  Kind = "hypothesis"
)

/*
OpportunitySummary is the dimensionless evidence balance for the graph's
explicit decision proposition. Conditions are reported separately and never
smuggled into directional support.
*/
type OpportunitySummary struct {
	Hypothesis    string
	Support       float64
	Contradiction float64
	Conditions    float64
	Balance       float64
	Confidence    float64
	Score         float64
	Direction     float64
	Ready         bool
}
