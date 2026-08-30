package advisor

import (
	"github.com/theapemachine/symm/types"
)

/*
CoordinationSupportBindings declares the inference/support facts behind a
coordination reading: sample counts, p-values, standard errors, and search
provenance from Correlation and LeadLag. They are descriptive, never
directional, and must never be multiplied into market direction. Their purpose
is to let a consumer distinguish "measured relationship with good support" from
"same numeric relationship under weak/search-heavy evidence".
*/
func CoordinationSupportBindings() []MetricBinding {
	return []MetricBinding{
		NewMetricBinding("correlation", "effective_sample_count", "advisor/coordination_support/eff_sample_count"),
		NewMetricBinding("correlation", "overlap_pair_count", "advisor/coordination_support/overlap_pair_count"),
		NewMetricBinding("correlation", "correlation_p_value", "advisor/coordination_support/correlation_p_value"),
		NewMetricBinding("correlation", "correlation_standard_error_fisher", "advisor/coordination_support/correlation_se_fisher"),
		NewMetricBinding("leadlag", "effective_sample_count", "advisor/coordination_support/lag_eff_sample_count"),
		NewMetricBinding("leadlag", "search_count", "advisor/coordination_support/lag_search_count"),
		NewMetricBinding("leadlag", "correlation_p_value", "advisor/coordination_support/lag_p_value"),
		NewMetricBinding("leadlag", "search_adjusted_p_value", "advisor/coordination_support/lag_search_adjusted_p"),
		NewMetricBinding("leadlag", "lag_peak_prominence", "advisor/coordination_support/lag_peak_prominence"),
		NewMetricBinding("leadlag", "lag_peak_curvature", "advisor/coordination_support/lag_peak_curvature"),
		NewMetricBinding("leadlag", "lag_search_resolution_seconds", "advisor/coordination_support/lag_search_resolution"),
		NewMetricBinding("leadlag", "lag_search_span", "advisor/coordination_support/lag_search_span"),
	}
}

/*
NewCoordinationSupportAdvisor constructs the KindCoordinationSupport Advisor:
the inference/support provenance behind coordination readings, surfaced without
pretending they predict direction.
*/
func NewCoordinationSupportAdvisor(name string) *Advisor {
	return newLatestFactsAdvisor(name, types.KindCoordinationSupport, CoordinationSupportBindings())
}
