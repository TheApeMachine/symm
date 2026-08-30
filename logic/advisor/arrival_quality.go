package advisor

import (
	"github.com/theapemachine/symm/types"
)

/*
ArrivalQualityBindings declares the Hawkes model-behaviour / epistemic context:
how the fitted arrival model is behaving and how much its interpretation should
be trusted. It is model/quality context, NOT directional market evidence.

The Hawkes signal does not emit a `spectral_radius_velocity` coordinate in the
current implementation, so the branching spectral radius is surfaced directly
and the velocity slot is omitted rather than fabricated. Likelihood gains,
standardized innovations, compensators, and expected descendants are retained
as fit diagnostics and model state; they must never feed direction, be
converted into "danger", or become confidence points.
*/
func ArrivalQualityBindings() []MetricBinding {
	return []MetricBinding{
		NewMetricBinding("hawkes", "branching_spectral_radius", "advisor/arrival_quality/branching_spectral_radius"),
		NewMetricBinding("hawkes", "log_likelihood_gain_vs_poisson", "advisor/arrival_quality/ll_gain_poisson"),
		NewMetricBinding("hawkes", "log_likelihood_gain_vs_self_only", "advisor/arrival_quality/ll_gain_self"),
		NewMetricBinding("hawkes", "log_likelihood_gain_per_event_vs_poisson", "advisor/arrival_quality/ll_gain_poisson_per_event"),
		NewMetricBinding("hawkes", "log_likelihood_gain_per_event_vs_self_only", "advisor/arrival_quality/ll_gain_self_per_event"),
		NewMetricBinding("hawkes", "compensator:buy", "advisor/arrival_quality/compensator_buy"),
		NewMetricBinding("hawkes", "compensator:sell", "advisor/arrival_quality/compensator_sell"),
		NewMetricBinding("hawkes", "standardized_innovation:buy", "advisor/arrival_quality/innovation_buy"),
		NewMetricBinding("hawkes", "standardized_innovation:sell", "advisor/arrival_quality/innovation_sell"),
		NewMetricBinding("hawkes", "expected_descendants_from_buy", "advisor/arrival_quality/descendants_buy"),
		NewMetricBinding("hawkes", "expected_descendants_from_sell", "advisor/arrival_quality/descendants_sell"),
	}
}

/*
NewArrivalQualityAdvisor constructs the KindArrivalQuality Advisor: the fitted
arrival model's behaviour and trustworthiness, surfaced without turning model
fit into directional conclusions.
*/
func NewArrivalQualityAdvisor(name string) *Advisor {
	return newLatestFactsAdvisor(name, types.KindArrivalQuality, ArrivalQualityBindings())
}
