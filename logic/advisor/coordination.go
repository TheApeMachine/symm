package advisor

import (
	"github.com/theapemachine/symm/types"
)

/*
CoordinationBindings declares the cohort price-path coordination facts from
Correlation and LeadLag. These signals emit cohort reductions (not pair-specific
outputs), so the Perspective's Peer stays empty; no peer identity is fabricated.

Temporal precedence is never labeled causality or leadership: these are
dependence and alignment geometry, nothing more.
*/
func CoordinationBindings() []MetricBinding {
	return []MetricBinding{
		NewMetricBinding("correlation", "signed_correlation", "advisor/coordination/signed_correlation"),
		NewMetricBinding("correlation", "correlation_divergence", "advisor/coordination/correlation_divergence"),
		NewMetricBinding("correlation", "correlation_zscore", "advisor/coordination/correlation_zscore"),
		NewMetricBinding("correlation", "relative_return_energy", "advisor/coordination/relative_return_energy"),
		NewMetricBinding("leadlag", "contemporaneous_correlation", "advisor/coordination/contemporaneous_correlation"),
		NewMetricBinding("leadlag", "best_lag_correlation", "advisor/coordination/best_lag_correlation"),
		NewMetricBinding("leadlag", "best_lag_seconds", "advisor/coordination/best_lag_seconds"),
		NewMetricBinding("leadlag", "absolute_correlation_gain", "advisor/coordination/absolute_correlation_gain"),
		NewMetricBinding("leadlag", "lag_zscore", "advisor/coordination/lag_zscore"),
		NewMetricBinding("leadlag", "correlation_gain_zscore", "advisor/coordination/correlation_gain_zscore"),
		NewMetricBinding("leadlag", "best_lag_correlation_zscore", "advisor/coordination/best_lag_correlation_zscore"),
		NewMetricBinding("leadlag", "lag_velocity", "advisor/coordination/lag_velocity"),
	}
}

/*
NewCoordinationAdvisor constructs the KindCoordination Advisor: cohort price-
path coupling and lag geometry, surfaced verbatim.
*/
func NewCoordinationAdvisor(name string) *Advisor {
	return newLatestFactsAdvisor(name, types.KindCoordination, CoordinationBindings())
}
