package advisor

import (
	"github.com/theapemachine/symm/types"
)

/*
MorphologyBindings declares the static/current dimensionless book-shape facts:
side concentration and side entropy. These describe arrangement, never
manipulation or intent.
*/
func MorphologyBindings() []MetricBinding {
	return []MetricBinding{
		NewMetricBinding("morphology", "concentration:bid", "advisor/morphology/concentration_bid"),
		NewMetricBinding("morphology", "concentration:ask", "advisor/morphology/concentration_ask"),
		NewMetricBinding("morphology", "entropy:bid", "advisor/morphology/entropy_bid"),
		NewMetricBinding("morphology", "entropy:ask", "advisor/morphology/entropy_ask"),
	}
}

/*
NewMorphologyAdvisor constructs the KindMorphology Advisor: current book-shape
arrangement (concentration and entropy per side), surfaced verbatim.
*/
func NewMorphologyAdvisor(name string) *Advisor {
	return newLatestFactsAdvisor(name, types.KindMorphology, MorphologyBindings())
}
