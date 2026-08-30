package advisor

import (
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
ExecutionBindings declares the executed-flow fact and the two displayed-capacity
facts the Execution Context Advisor composes for one symbol:

  - cvd/signed_net_fraction_zscore — executed flow: aggressor-side imbalance,
    already causally standardized against its own history by the CVD signal.
  - liquidity/touch_notional_imbalance — displayed capacity asymmetry: the sign
    and size of bid/ask touch notional tilt.
  - liquidity/relative_spread — the cost of crossing the touch, dimensionless.

Every metric is already causally standardized or dimensionless as published by
its own signal, so this Advisor performs no renormalization: recomputing a
second normalization universe here would silently redefine what "aggressive" or
"thin" means without the caller's knowledge.
*/
func ExecutionBindings() []MetricBinding {
	return []MetricBinding{
		NewMetricBinding("cvd", "signed_net_fraction_zscore", "advisor/execution/flow"),
		NewMetricBinding("liquidity", "touch_notional_imbalance", "advisor/execution/imbalance"),
		NewMetricBinding("liquidity", "relative_spread", "advisor/execution/spread"),
	}
}

/*
ExecutionPipeline projects each bound metric's raw value into its own output
slot, gated per binding on Fresh. It is a joint-facts composition, not a score:
the two capacity facts and the flow fact are carried side by side, each with its
own definedness, so a consumer reads "flow is +σ while ask-side touch is thin
and spread is wide" — never a single multiplied scalar. The METRIC_MAP forbids
collapsing the conditional meaning of flow-under-shallow-liquidity into
`flow * liquidityConfidence`; a joint reading preserves the condition instead of
erasing it.

ForkStrict composes the branches for the same reason LiquidityPipeline does:
two bindings fresh on the same Measurement each return a copy of the shared input
including the other's prior state, and a blind overlay would revert the first
branch back to stale. scrubFresh removes every marker before commit so a Fresh
bit set once can never read as fresh again.
*/
func ExecutionPipeline(bindings []MetricBinding) nmtypes.Primitive {
	branches := make([]nmtypes.Primitive, 0, len(bindings))

	for _, binding := range bindings {
		branches = append(branches, jointFact(binding))
	}

	return nmtypes.Pipe(nmtypes.ForkStrict(branches...), scrubFresh(bindings))
}

/*
jointFact relays one binding's projected raw value into that binding's output
slot, gated on Fresh. It is the joint-facts analogue of freshTemporalContext:
the metric's already-defined value is carried forward unchanged, with its own
provenance, rather than run through a derived-statistic stage.
*/
func jointFact(binding MetricBinding) nmtypes.Primitive {
	return func(input *nmtypes.Frame) {
		if !input.Has(binding.Fresh) {
			return
		}

		value, found := input.Get(binding.Series.ValueSymbol)

		if !found {
			return
		}

		input.Put(binding.Series.ValueSymbol, value)
	}
}

/*
ExecutionOutputs declares the three named joint facts the pipeline emits: the
executed-flow z-score, the touch-notional imbalance, and the relative spread.
Each borrows its bound metric's own Maturity/SNR/SNRDefined provenance through
NewMetricOutput because each fact is exactly one measurement's honest reading,
not a derived combination.
*/
func ExecutionOutputs(bindings []MetricBinding) []Output {
	outputs := make([]Output, 0, len(bindings))

	for _, binding := range bindings {
		outputs = append(outputs, NewMetricOutput(binding.Series.ValueSymbol, binding))
	}

	return outputs
}

/*
NewExecutionAdvisor constructs the single concrete execution-context Advisor
instance over ExecutionPipeline and ExecutionBindings. It answers the named
question: what does the current executed flow mean against the displayed
capacity it executed into — flow, capacity asymmetry, and crossing cost presented
jointly, never reduced to one score.
*/
func NewExecutionAdvisor(name string) *Advisor {
	bindings := ExecutionBindings()

	return NewAdvisor(
		name,
		types.KindExecutionContext,
		ExecutionPipeline(bindings),
		bindings,
		ExecutionOutputs(bindings),
	)
}
