package advisor

import (
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
LiquidityBindings declares the three execution-terrain metrics the Liquidity
Advisor composes for one symbol: relative spread and touch notional imbalance
from the "liquidity" signal, and book imbalance from the "depthflow" signal.
Each binding gets its own series prefix so the three temporal-context streams
occupy distinct Frame slots inside the one committed per-symbol state.
*/
func LiquidityBindings() []MetricBinding {
	return []MetricBinding{
		NewMetricBinding("liquidity", "relative_spread", "advisor/liquidity/relative_spread"),
		NewMetricBinding("liquidity", "touch_notional_imbalance", "advisor/liquidity/touch_notional_imbalance"),
		NewMetricBinding("depthflow", "book_imbalance", "advisor/liquidity/book_imbalance"),
	}
}

/*
LiquidityPipeline composes the temporal context (current value, adaptive
baseline, departure z-score, first difference) of every bound metric,
independently, into one Frame. Each metric's stage runs under its own series
prefix, so the three streams never collide, and TryFork tolerates a metric
that has not been observed yet: a branch whose own required value has never
arrived fails before writing anything and is dropped, while every branch that
has data still composes. This is what lets a liquidity Measurement and a later
depthflow Measurement for the same symbol both contribute to the same
committed state without either erasing the other.
*/
func LiquidityPipeline(bindings []MetricBinding) nmtypes.Primitive {
	branches := make([]nmtypes.Primitive, 0, len(bindings))

	for _, binding := range bindings {
		branches = append(branches, temporalContext(binding.Prefix))
	}

	return nmtypes.TryFork(branches...)
}

/*
temporalContext returns the Window→ZScore→Baseline→Velocity composition for
one series prefix: the current value, adaptive baseline, departure z-score,
and first difference of that series' own event-time history.
*/
func temporalContext(prefix string) nmtypes.Primitive {
	return nmtypes.Pipe(
		temporal.Window(prefix),
		statistic.ZScore(prefix),
		statistic.Baseline(prefix),
		statistic.Velocity(prefix),
	)
}

/*
NewLiquidityAdvisor constructs the single concrete KindLiquidity Advisor
instance over LiquidityPipeline and LiquidityBindings.
*/
func NewLiquidityAdvisor(name string) *Advisor {
	bindings := LiquidityBindings()

	return NewAdvisor(name, types.KindLiquidity, LiquidityPipeline(bindings), bindings...)
}
