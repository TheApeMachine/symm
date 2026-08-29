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
other bound metric's event.

A not-fresh branch is a deliberate no-op (it returns its input unchanged, no
error), never a failure to be forgiven: Fresh already tells us exactly which
branches apply to this event, so there is nothing left to infer from whether a
branch happened to change the frame — no need for TryFork's absence-vs-defect
guessing here.

ForkStrict, not plain Fork, composes the branches: when two or more bindings
are fresh on the same Measurement, each branch's returned frame is a full copy
of the shared input, including every other binding's already-populated prior
state, untouched. Plain Fork overlays each branch's output onto the composed
result unconditionally and in order, so a later branch's untouched copy of an
earlier branch's freshly mutated slot silently reverts it — a real,
previously undetected clobbering bug that surfaces exactly when one
Measurement carries two or more bound metrics at once. ForkStrict compares
each branch's output against the shared input to isolate only what that
branch actually changed before merging, so two branches that each mutate only
their own series never step on one another regardless of merge order.
*/
func LiquidityPipeline(bindings []MetricBinding) nmtypes.Primitive {
	branches := make([]nmtypes.Primitive, 0, len(bindings))

	for _, binding := range bindings {
		branches = append(branches, freshTemporalContext(binding))
	}

	return nmtypes.Pipe(nmtypes.ForkStrict(branches...), scrubFresh(bindings))
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
			NewMetricOutput(binding.Series.ValueSymbol, binding),
			NewMetricOutput(nmtypes.MustIntern(temporal.JoinPrefix(binding.Prefix, "baseline/value")), binding),
			NewMetricOutput(nmtypes.MustIntern(temporal.JoinPrefix(binding.Prefix, "z/value")), binding),
			NewMetricOutput(nmtypes.MustIntern(temporal.JoinPrefix(binding.Prefix, "velocity/delta")), binding),
		)
	}

	return outputs
}

/*
freshTemporalContext returns the Window→ZScore→Baseline→Velocity composition
for one binding's series prefix, gated on that binding's Fresh marker: the
stage only advances the series when this call's own Measurement delivered the
value. When it did not, this is a deliberate no-op — the frame returns exactly
as given, with no error — never a condition for Fork to forgive.
*/
func freshTemporalContext(binding MetricBinding) nmtypes.Primitive {
	stage := nmtypes.Pipe(
		temporal.Window(binding.Prefix),
		statistic.ZScore(binding.Prefix),
		statistic.Baseline(binding.Prefix),
		statistic.Velocity(binding.Prefix),
	)

	return func(input *nmtypes.Frame) {
		if !input.Has(binding.Fresh) {
			return
		}

		stage(input)
	}
}

/*
scrubFresh deletes every binding's Fresh marker from the frame Fork hands
back. Fork's output starts as a copy of its input (which still has whichever
markers the caller set) and only overlays what each branch newly wrote, so a
marker present in the input survives untouched unless something deletes it
from the composed output directly — a branch simply passing its input through
unchanged (the not-fresh no-op case) does not clear it either. Without this
final scrub a Fresh bit set once would commit and read as fresh again on every
later call regardless of what that call actually delivered, permanently
defeating the gate after its first success.
*/
func scrubFresh(bindings []MetricBinding) nmtypes.Primitive {
	return func(input *nmtypes.Frame) {
		for _, binding := range bindings {
			input.Delete(binding.Fresh)
		}
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
