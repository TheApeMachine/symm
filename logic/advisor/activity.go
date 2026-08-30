package advisor

import (
	"github.com/theapemachine/symm/types"
)

/*
ActivityBindings declares the volume-clock activity facts — the legitimate use
of the legacy "pumpdump" signal. These describe completed economic throughput,
volume-bar geometry, and midpoint response. No "pump", "dump", ignition,
exhaustion, manipulation, or continuation conclusion is emitted from them, and
the duplicate spread metrics stay out: Liquidity owns spread state.
*/
func ActivityBindings() []MetricBinding {
	return []MetricBinding{
		NewMetricBinding("pumpdump", "volume_bar_quantity", "advisor/activity/volume_bar_quantity"),
		NewMetricBinding("pumpdump", "volume_bar_notional", "advisor/activity/volume_bar_notional"),
		NewMetricBinding("pumpdump", "volume_bar_trade_count", "advisor/activity/volume_bar_trade_count"),
		NewMetricBinding("pumpdump", "volume_bar_duration", "advisor/activity/volume_bar_duration"),
		NewMetricBinding("pumpdump", "volume_rate", "advisor/activity/volume_rate"),
		NewMetricBinding("pumpdump", "notional_rate", "advisor/activity/notional_rate"),
		NewMetricBinding("pumpdump", "trade_rate", "advisor/activity/trade_rate"),
		NewMetricBinding("pumpdump", "notional_rate_zscore", "advisor/activity/notional_rate_zscore"),
		NewMetricBinding("pumpdump", "notional_rate_velocity", "advisor/activity/notional_rate_velocity"),
		NewMetricBinding("pumpdump", "midpoint_log_return", "advisor/activity/midpoint_log_return"),
		NewMetricBinding("pumpdump", "midpoint_return_rate", "advisor/activity/midpoint_return_rate"),
		NewMetricBinding("pumpdump", "midpoint_return_zscore", "advisor/activity/midpoint_return_zscore"),
	}
}

/*
NewActivityAdvisor constructs the KindActivity Advisor: volume-clock economic
activity and midpoint response, surfaced as facts without a directional
conclusion.
*/
func NewActivityAdvisor(name string) *Advisor {
	return newLatestFactsAdvisor(name, types.KindActivity, ActivityBindings())
}
