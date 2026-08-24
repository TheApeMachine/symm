package causal

import (
	"time"
)

/*
IdentificationStatus is the explicit state of a causal query. It is never
collapsed into a boolean "Ready": a non-identifiable causal question returns
non-identifiable, not zero, not correlation, and not the previous estimate.
*/
type IdentificationStatus uint8

const (
	// IdentificationIdentified means a defensible identification strategy
	// exists under the schema and the estimate is defined.
	IdentificationIdentified IdentificationStatus = iota
	// IdentificationNotIdentifiable means no valid adjustment/identification
	// strategy exists under the current schema.
	IdentificationNotIdentifiable
	// IdentificationUnsupportedTreatment means the treatment state has no
	// relevant observational support under comparable controls.
	IdentificationUnsupportedTreatment
	// IdentificationInsufficientRank means the design matrix lacks full rank.
	IdentificationInsufficientRank
	// IdentificationInsufficientSupport means there are too few effective
	// observations for the parameter count.
	IdentificationInsufficientSupport
	// IdentificationUndefined means the query state is mathematically undefined.
	IdentificationUndefined
)

func (status IdentificationStatus) String() string {
	switch status {
	case IdentificationIdentified:
		return "identified"
	case IdentificationNotIdentifiable:
		return "not_identifiable"
	case IdentificationUnsupportedTreatment:
		return "unsupported_treatment"
	case IdentificationInsufficientRank:
		return "insufficient_rank"
	case IdentificationInsufficientSupport:
		return "insufficient_support"
	case IdentificationUndefined:
		return "undefined"
	default:
		return "unknown"
	}
}

/*
OutcomeRequest is one explicit causal question.
*/
type OutcomeRequest struct {
	// Treatment is the strategic intervention name (wait, enter, exit, scale).
	Treatment string
	// Target is the outcome variable.
	Target VariableID
	// Reference is the reference intervention the effect is relative to.
	Reference string
	// Current is the current observed state of the variables the query needs.
	Current map[VariableID]float64
	// At is the event time of the query.
	At time.Time
}

/*
TreatmentEffect is the causal estimate contract. Unavailable uncertainty
remains unavailable; there is no silent fallback.
*/
type TreatmentEffect struct {
	Treatment  string
	Target     VariableID
	AdjustmentSet []VariableID
	IdentificationReasoning string

	ExpectedOutcome        float64
	EffectRelativeToReference float64
	ResidualNoise          float64
	StandardError          float64
	EffectiveSupport       float64
	Maturity               float64

	ModelVersion  string
	SchemaVersion uint64
	From          time.Time
	At            time.Time
	Status        IdentificationStatus
}

/*
Defined reports whether the estimate is identified.
*/
func (effect *TreatmentEffect) Defined() bool {
	return effect != nil && effect.Status == IdentificationIdentified
}

/*
PortfolioOutcome builds an identified deterministic portfolio effect. An
action directly changes only variables the strategy actually controls
(position, cash, order state); it never directly mutates market coordinates
without an explicit market-impact model.
*/
func (schema *CausalSchema) PortfolioOutcome(
	request OutcomeRequest,
	expectedValue float64,
	reasoning string,
) *TreatmentEffect {
	effect := &TreatmentEffect{
		Treatment:     request.Treatment,
		Target:        request.Target,
		AdjustmentSet: []VariableID{},
		IdentificationReasoning: reasoning,
		ExpectedOutcome: expectedValue,
		EffectRelativeToReference: expectedValue,
		At:            request.At,
		SchemaVersion: schema.Version,
		Status:        IdentificationIdentified,
	}

	return effect
}

/*
NotIdentifiableOutcome builds the explicit non-identifiable result. It never
substitutes correlation, predictive Influence, zero, or an old estimate.
*/
func (schema *CausalSchema) NotIdentifiableOutcome(
	request OutcomeRequest,
	reasoning string,
) *TreatmentEffect {
	return &TreatmentEffect{
		Treatment:     request.Treatment,
		Target:        request.Target,
		AdjustmentSet: []VariableID{},
		IdentificationReasoning: reasoning,
		At:            request.At,
		SchemaVersion: schema.Version,
		Status:        IdentificationNotIdentifiable,
	}
}
