package mcts

/*
IdentificationStatus is the search's own causal-identification provenance type.
A search result carries it so the caller can distinguish an identified estimate
from an explicitly unavailable one without reaching into the application's
causal package. It is structural: it reports whether a defensible identification
exists and why it does not, never a market judgment.

It lives here so the search engine is a leaf package with no dependency on the
application layer.
*/
type IdentificationStatus uint8

const (
	IdentificationIdentified IdentificationStatus = iota
	IdentificationNotIdentifiable
	IdentificationUnsupportedTreatment
	IdentificationInsufficientRank
	IdentificationInsufficientSupport
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
