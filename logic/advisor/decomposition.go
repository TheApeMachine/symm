package advisor

import (
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

// Decomposition derived-output symbols.
var (
	symbolDecompMeanNotional = nmtypes.MustIntern("advisor/decomposition/mean_event_notional")
	symbolDecompMeanMatur    = nmtypes.MustIntern("advisor/decomposition/mean_event_notional/maturity")
)

/*
DecompositionBindings declares the two quantities that jointly separate the
mechanisms a single raw "activity" number conflates:

  - hawkes/arrival_rate — event frequency: events per second, from the fitted
    arrival process.
  - cvd/gross_notional_rate — economic throughput: quote per second of
    executed flow.

Together they are the METRIC_MAP §5 cvd→hawkes DECOMPOSES relation: arrival
intensity × mean event notional separates event frequency from economic size.
*/
func DecompositionBindings() []MetricBinding {
	return []MetricBinding{
		NewMetricBinding("hawkes", "arrival_rate", "advisor/decomposition/frequency"),
		NewMetricBinding("cvd", "gross_notional_rate", "advisor/decomposition/throughput"),
	}
}

/*
DecompositionPipeline relays each bound metric's raw value into its own slot
(gated on Fresh) and then computes the actual decomposition the claim requires
but the previous implementation merely asserted: mean economic event size.

  mean_event_notional = gross_notional_rate / arrival_rate

with unit:

  (quote / second) / (events / second) = quote / event

i.e. mean economic event size, provided the arrival rate is valid and both
quantities are causally aligned. A zero or undefined arrival rate leaves the
derived quantity UNDEFINED — never zero, never infinity, never a fabricated
error (see ratioNamed). Frequency and throughput remain exposed as their own
facts so "many-small vs few-large" is a real combination of three numbers, not
two numbers copied beside each other and labeled a decomposition.
*/
func DecompositionPipeline(bindings []MetricBinding) nmtypes.Primitive {
	freq := bindings[0]
	throughput := bindings[1]

	return nmtypes.Pipe(
		nmtypes.ForkStrict(jointFact(freq), jointFact(throughput)),
		ratioNamed(
			throughput.Series.ValueSymbol, throughput.Series.SecSymbol, throughput.Series.NsecSymbol,
			freq.Series.ValueSymbol, freq.Series.SecSymbol, freq.Series.NsecSymbol,
			throughput.Maturity, freq.Maturity,
			symbolDecompMeanNotional, symbolDecompMeanMatur,
		),
		scrubFresh(bindings),
	)
}

/*
DecompositionOutputs declares the derived mean event size plus the two input
facts (frequency and throughput) that the decomposition separates. The derived
quantity carries its own min-maturity provenance; the two inputs borrow their
bound metrics' own provenance.
*/
func DecompositionOutputs(bindings []MetricBinding) []Output {
	return []Output{
		NewDerivedOutput(symbolDecompMeanNotional, symbolDecompMeanMatur),
		NewMetricOutput(bindings[0].Series.ValueSymbol, bindings[0]), // arrival_rate
		NewMetricOutput(bindings[1].Series.ValueSymbol, bindings[1]), // gross_notional_rate
	}
}

/*
NewDecompositionAdvisor constructs the decomposition Advisor instance. It
answers the named question: is the current executed activity many-small trades
or few-large trades — arrival frequency and mean economic event size computed
from a real division, never reduced to two uncombined numbers.
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
