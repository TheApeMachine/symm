package advisor

import (
	"github.com/theapemachine/symm/types"
)

/*
LiquidityBindings declares the current liquidity-state facts the Liquidity
Advisor composes for one symbol. Every metric is the Liquidity signal's own
already-derived reading; the advisor performs no re-estimation. The prior
temporal-context pipeline (Window→ZScore→Baseline→Velocity) has been removed:
the signal already publishes the canonical current state, and maintaining a
second independent estimator would silently redefine the signal's own
baselines/z-scores.
*/
func LiquidityBindings() []MetricBinding {
	return []MetricBinding{
		NewMetricBinding("liquidity", "relative_spread", "advisor/liquidity/relative_spread"),
		NewMetricBinding("liquidity", "touch_notional:bid", "advisor/liquidity/touch_notional_bid"),
		NewMetricBinding("liquidity", "touch_notional:ask", "advisor/liquidity/touch_notional_ask"),
		NewMetricBinding("liquidity", "two_sided_touch_notional", "advisor/liquidity/two_sided_touch_notional"),
		NewMetricBinding("liquidity", "touch_notional_imbalance", "advisor/liquidity/touch_notional_imbalance"),
		NewMetricBinding("liquidity", "depth_ratio:bid", "advisor/liquidity/depth_ratio_bid"),
		NewMetricBinding("liquidity", "depth_ratio:ask", "advisor/liquidity/depth_ratio_ask"),
		NewMetricBinding("liquidity", "spread_ratio", "advisor/liquidity/spread_ratio"),
	}
}

/*
NewLiquidityAdvisor constructs the KindLiquidity Advisor: the current displayed
liquidity state (touch notionals, two-sided capacity, imbalance, depth ratios,
spread and relative spread) surfaced from the Liquidity signal's own readings.
*/
func NewLiquidityAdvisor(name string) *Advisor {
	return newLatestFactsAdvisor(name, types.KindLiquidity, LiquidityBindings())
}
