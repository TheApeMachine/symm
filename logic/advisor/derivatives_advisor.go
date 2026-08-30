package advisor

import (
	"github.com/theapemachine/symm/types"
)

/*
DerivativesBindings declares the leverage/basis/OI/liquidation/decoupling
facts. The futures ingress already canonicalizes futures products to the spot
symbol, so ticker and trade Derivatives measurements for one canonical spot
symbol compose into one resident Perspective. This is context, not a direction
signal.
*/
func DerivativesBindings() []MetricBinding {
	return []MetricBinding{
		NewMetricBinding("derivatives", "basis", "advisor/derivatives/basis"),
		NewMetricBinding("derivatives", "basis_zscore", "advisor/derivatives/basis_zscore"),
		NewMetricBinding("derivatives", "basis_rate", "advisor/derivatives/basis_rate"),
		NewMetricBinding("derivatives", "open_interest", "advisor/derivatives/open_interest"),
		NewMetricBinding("derivatives", "open_interest_growth_rate", "advisor/derivatives/open_interest_growth_rate"),
		NewMetricBinding("derivatives", "open_interest_growth_zscore", "advisor/derivatives/open_interest_growth_zscore"),
		NewMetricBinding("derivatives", "open_interest_growth_velocity", "advisor/derivatives/open_interest_growth_velocity"),
		NewMetricBinding("derivatives", "return_gap", "advisor/derivatives/return_gap"),
		NewMetricBinding("derivatives", "return_gap_zscore", "advisor/derivatives/return_gap_zscore"),
		NewMetricBinding("derivatives", "gross_liquidation_notional", "advisor/derivatives/gross_liquidation_notional"),
		NewMetricBinding("derivatives", "net_liquidation_notional", "advisor/derivatives/net_liquidation_notional"),
		NewMetricBinding("derivatives", "liquidation_share", "advisor/derivatives/liquidation_share"),
	}
}

/*
NewDerivativesAdvisor constructs the KindDerivatives Advisor: basis/OI/
liquidation/decoupling context, surfaced verbatim.
*/
func NewDerivativesAdvisor(name string) *Advisor {
	return newLatestFactsAdvisor(name, types.KindDerivatives, DerivativesBindings())
}
