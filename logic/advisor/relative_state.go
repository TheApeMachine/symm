package advisor

import (
	"github.com/theapemachine/symm/types"
)

/*
RelativeStateBindings declares the cross-sectional price-state facts. The
package is named "sentiment" but its meaning is cohort price state — breadth,
dispersion, directional consensus — not social/news sentiment. The Perspective
tells a consumer whether the focal move is broad, narrow, consensual, dispersed,
or unusual relative to the cohort, without turning breadth into RiskOn/RiskOff
gates.
*/
func RelativeStateBindings() []MetricBinding {
	return []MetricBinding{
		NewMetricBinding("sentiment", "return", "advisor/relative_state/return"),
		NewMetricBinding("sentiment", "advance_fraction", "advisor/relative_state/advance_fraction"),
		NewMetricBinding("sentiment", "decline_fraction", "advisor/relative_state/decline_fraction"),
		NewMetricBinding("sentiment", "directional_participation", "advisor/relative_state/directional_participation"),
		NewMetricBinding("sentiment", "directional_agreement", "advisor/relative_state/directional_agreement"),
		NewMetricBinding("sentiment", "directional_consensus", "advisor/relative_state/directional_consensus"),
		NewMetricBinding("sentiment", "breadth", "advisor/relative_state/breadth"),
		NewMetricBinding("sentiment", "breadth_zscore", "advisor/relative_state/breadth_zscore"),
		NewMetricBinding("sentiment", "median_return", "advisor/relative_state/median_return"),
		NewMetricBinding("sentiment", "median_absolute_return", "advisor/relative_state/median_absolute_return"),
		NewMetricBinding("sentiment", "median_absolute_return_zscore", "advisor/relative_state/median_absolute_return_zscore"),
		NewMetricBinding("sentiment", "return_interquartile_range", "advisor/relative_state/return_interquartile_range"),
	}
}

/*
NewRelativeStateAdvisor constructs the KindRelativeState Advisor: cross-
sectional cohort price state, surfaced as continuous facts.
*/
func NewRelativeStateAdvisor(name string) *Advisor {
	return newLatestFactsAdvisor(name, types.KindRelativeState, RelativeStateBindings())
}
