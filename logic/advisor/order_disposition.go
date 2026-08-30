package advisor

import (
	"github.com/theapemachine/symm/types"
)

/*
OrderDispositionBindings declares the Toxicity disposition facts, side-paired
so a consumer sees what happened to each side of displayed touch liquidity
independently. The raw touch fill fraction is the sample the signal emits
("touch_fill_fraction"), and the other mechanisms (net withdrawal, net
replenishment, retreat) are their signal-defined fractions and z-scores.

The four mechanisms are deliberately NOT collapsed into a toxicity score: a
fill, an unexplained withdrawal, a replenishment, and a price retreat are
mechanically different facts and must remain distinguishable. An increase in
withdrawal does not fabricate a decrease in fill unless the signal itself
reports one.
*/
func OrderDispositionBindings() []MetricBinding {
	return []MetricBinding{
		NewMetricBinding("toxicity", "touch_fill_fraction:bid", "advisor/order_disposition/fill_bid"),
		NewMetricBinding("toxicity", "touch_fill_fraction:ask", "advisor/order_disposition/fill_ask"),
		NewMetricBinding("toxicity", "net_withdrawal_fraction:bid", "advisor/order_disposition/withdrawal_bid"),
		NewMetricBinding("toxicity", "net_withdrawal_fraction:ask", "advisor/order_disposition/withdrawal_ask"),
		NewMetricBinding("toxicity", "net_replenishment_fraction:bid", "advisor/order_disposition/replenishment_bid"),
		NewMetricBinding("toxicity", "net_replenishment_fraction:ask", "advisor/order_disposition/replenishment_ask"),
		NewMetricBinding("toxicity", "retreat_fraction:bid", "advisor/order_disposition/retreat_bid"),
		NewMetricBinding("toxicity", "retreat_fraction:ask", "advisor/order_disposition/retreat_ask"),
		NewMetricBinding("toxicity", "withdrawal_fraction_zscore:bid", "advisor/order_disposition/withdrawal_zscore_bid"),
		NewMetricBinding("toxicity", "withdrawal_fraction_zscore:ask", "advisor/order_disposition/withdrawal_zscore_ask"),
		NewMetricBinding("toxicity", "retreat_fraction_zscore:bid", "advisor/order_disposition/retreat_zscore_bid"),
		NewMetricBinding("toxicity", "retreat_fraction_zscore:ask", "advisor/order_disposition/retreat_zscore_ask"),
	}
}

/*
NewOrderDispositionAdvisor constructs the KindOrderDisposition Advisor: what
happened to displayed touch liquidity on each side, retained as distinguishable
mechanisms for Flow/Liquidity interpretation and PositionRisk.
*/
func NewOrderDispositionAdvisor(name string) *Advisor {
	return newLatestFactsAdvisor(name, types.KindOrderDisposition, OrderDispositionBindings())
}
