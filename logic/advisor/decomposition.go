package advisor

import (
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
DecompositionBindings declares the two quantities that jointly separate the
mechanisms a single raw "activity" number conflates:

  - hawkes/arrival_rate — event frequency: trades per second, from the fitted
    arrival process.
  - cvd/gross_notional_rate — economic throughput: notional per second of
    executed flow.

Presented jointly, a consumer can tell "many small trades" (high frequency,
modest throughput) from "few large trades" (low frequency, high throughput) —
the METRIC_MAP §5 cvd→hawkes DECOMPOSES edge: arrival intensity × mean trade
notional separates event frequency from economic event size. The two facts are
kept separate rather than folded into one throughput number, so neither
mechanism is erased.
*/
func DecompositionBindings() []MetricBinding {
	return []MetricBinding{
		NewMetricBinding("hawkes", "arrival_rate", "advisor/decomposition/frequency"),
		NewMetricBinding("cvd", "gross_notional_rate", "advisor/decomposition/throughput"),
	}
}

/*
DecompositionPipeline projects each bound metric's raw value into its own
output slot, gated per binding on Fresh, using the same joint-facts composition
as ExecutionPipeline. No division, no multiplication: frequency and throughput
are carried side by side with their own definedness, so the decomposition is a
property the consumer reads, not a scalar this Advisor computes.
*/
func DecompositionPipeline(bindings []MetricBinding) nmtypes.Primitive {
	branches := make([]nmtypes.Primitive, 0, len(bindings))

	for _, binding := range bindings {
		branches = append(branches, jointFact(binding))
	}

	return nmtypes.Pipe(nmtypes.ForkStrict(branches...), scrubFresh(bindings))
}

/*
DecompositionOutputs declares the two named joint facts: event frequency and
economic throughput, each borrowing its bound metric's own provenance.
*/
func DecompositionOutputs(bindings []MetricBinding) []Output {
	outputs := make([]Output, 0, len(bindings))

	for _, binding := range bindings {
		outputs = append(outputs, NewMetricOutput(binding.Series.ValueSymbol, binding))
	}

	return outputs
}

/*
NewDecompositionAdvisor constructs the decomposition Advisor instance. It
answers the named question: is the current executed activity many-small trades
or few-large trades — frequency and economic size presented jointly, never
reduced to a single throughput scalar.
*/
func NewDecompositionAdvisor(name string) *Advisor {
	bindings := DecompositionBindings()

	return NewAdvisor(
		name,
		types.KindDecomposition,
		DecompositionPipeline(bindings),
		bindings,
		DecompositionOutputs(bindings),
	)
}
