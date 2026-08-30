package advisor

import (
	"github.com/theapemachine/symm/types"
)

/*
LiquidityDynamicsBindings declares the Liquidity signal's own causal
historical/dynamic state. Every metric is already estimated by the signal;
the advisor performs no Advisor-local Window/Baseline/ZScore/Velocity pass and
no re-estimation. The signal's noise/SNR/support facts remain epistemic
provenance — they are carried as each reading's Maturity/SNR, never turned into
directional readings or confidence multipliers.
*/
func LiquidityDynamicsBindings() []MetricBinding {
	return []MetricBinding{
		NewMetricBinding("liquidity", "touch_notional_baseline:bid", "advisor/liquidity_dynamics/touch_notional_baseline_bid"),
		NewMetricBinding("liquidity", "touch_notional_baseline:ask", "advisor/liquidity_dynamics/touch_notional_baseline_ask"),
		NewMetricBinding("liquidity", "relative_spread_baseline", "advisor/liquidity_dynamics/relative_spread_baseline"),
		NewMetricBinding("liquidity", "depth_divergence:bid", "advisor/liquidity_dynamics/depth_divergence_bid"),
		NewMetricBinding("liquidity", "depth_divergence:ask", "advisor/liquidity_dynamics/depth_divergence_ask"),
		NewMetricBinding("liquidity", "depth_zscore:bid", "advisor/liquidity_dynamics/depth_zscore_bid"),
		NewMetricBinding("liquidity", "depth_zscore:ask", "advisor/liquidity_dynamics/depth_zscore_ask"),
		NewMetricBinding("liquidity", "spread_divergence", "advisor/liquidity_dynamics/spread_divergence"),
		NewMetricBinding("liquidity", "spread_zscore", "advisor/liquidity_dynamics/spread_zscore"),
		NewMetricBinding("liquidity", "divergence_velocity:bid", "advisor/liquidity_dynamics/divergence_velocity_bid"),
		NewMetricBinding("liquidity", "divergence_velocity:ask", "advisor/liquidity_dynamics/divergence_velocity_ask"),
		NewMetricBinding("liquidity", "spread_divergence_velocity", "advisor/liquidity_dynamics/spread_divergence_velocity"),
	}
}

/*
NewLiquidityDynamicsAdvisor constructs the KindLiquidityDynamics Advisor: the
Liquidity signal's causal historical depth/spread state — baselines, log
divergences, z-scores, and divergence velocities — surfaced verbatim.
*/
func NewLiquidityDynamicsAdvisor(name string) *Advisor {
	return newLatestFactsAdvisor(name, types.KindLiquidityDynamics, LiquidityDynamicsBindings())
}
