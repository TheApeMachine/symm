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
prefix, so the three streams never collide.

Number.Step merges the previous committed Frame under the incoming one before
running the pipeline, so every branch sees every bound metric's retained value
and event time on every call, whether or not this call's Measurement actually
carried that metric. Each branch is therefore gated on the binding's Fresh
marker — populated by Advisor.Step only for the metrics this specific
Measurement delivered — so a branch advances its series exactly once per
genuine observation instead of re-appending the same retained sample on every
other bound metric's event. TryFork additionally tolerates a metric that has
never been observed at all: a branch whose own required value has never
arrived fails before writing anything and is dropped, while every branch that
has data still composes. Together these are what let a liquidity Measurement
and a later depthflow Measurement for the same symbol both contribute to the
same committed state without either erasing or duplicating the other.
*/
func LiquidityPipeline(bindings []MetricBinding) nmtypes.Primitive {
	branches := make([]nmtypes.Primitive, 0, len(bindings))

	for _, binding := range bindings {
		branches = append(branches, freshTemporalContext(binding))
	}

	return nmtypes.Pipe(nmtypes.TryFork(branches...), scrubFresh(bindings))
}

/*
LiquidityOutputs declares the four named facts LiquidityPipeline emits per
bound metric: its current value, adaptive baseline, departure z-score, and
first difference. This naming knowledge belongs to the Liquidity pipeline
alone — Advisor never assumes any pipeline produces these specific facts.
*/
func LiquidityOutputs(bindings []MetricBinding) []Output {
	outputs := make([]Output, 0, len(bindings)*4)

	for _, binding := range bindings {
		outputs = append(outputs,
			Output{Slot: binding.Series.ValueSymbol, Metric: binding},
			Output{Slot: nmtypes.MustIntern(temporal.JoinPrefix(binding.Prefix, "baseline/value")), Metric: binding},
			Output{Slot: nmtypes.MustIntern(temporal.JoinPrefix(binding.Prefix, "z/value")), Metric: binding},
			Output{Slot: nmtypes.MustIntern(temporal.JoinPrefix(binding.Prefix, "velocity/delta")), Metric: binding},
		)
	}

	return outputs
}

/*
freshTemporalContext returns the Window→ZScore→Baseline→Velocity composition
for one binding's series prefix, gated on that binding's Fresh marker: the
stage only advances the series when this call's own Measurement delivered the
value.
*/
func freshTemporalContext(binding MetricBinding) nmtypes.Primitive {
	stage := nmtypes.Pipe(
		temporal.Window(binding.Prefix),
		statistic.ZScore(binding.Prefix),
		statistic.Baseline(binding.Prefix),
		statistic.Velocity(binding.Prefix),
	)

	return func(input nmtypes.Frame) nmtypes.Frame {
		if !input.Has(binding.Fresh) {
			input.Err = nmtypes.PrimitiveError("advisor: binding is not fresh this step")

			return input
		}

		return stage(input)
	}
}

/*
scrubFresh deletes every binding's Fresh marker from the frame TryFork hands
back. A branch's own Delete only clears the marker from that branch's own
returned frame; TryFork's output starts as a copy of its input (which still
has whichever markers the caller set) and only overlays what each branch
newly wrote, so a marker present in the input survives untouched unless
something deletes it from the composed output directly. Without this final
scrub a Fresh bit set once would commit and read as fresh again on every
later call regardless of what that call actually delivered, permanently
defeating the gate after its first success.
*/
func scrubFresh(bindings []MetricBinding) nmtypes.Primitive {
	return func(input nmtypes.Frame) nmtypes.Frame {
		for _, binding := range bindings {
			input.Delete(binding.Fresh)
		}

		return input
	}
}

/*
NewLiquidityAdvisor constructs the single concrete KindLiquidity Advisor
instance over LiquidityPipeline and LiquidityBindings.
*/
func NewLiquidityAdvisor(name string) *Advisor {
	bindings := LiquidityBindings()

	return NewAdvisor(
		name,
		types.KindLiquidity,
		LiquidityPipeline(bindings),
		bindings,
		LiquidityOutputs(bindings),
	)
}
