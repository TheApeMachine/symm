package advisor

import (
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
latestFactsPipeline is the "latest composed facts" pipeline for Advisors that
merely retain already-derived signal facts and expose them as a Perspective.
Each bound metric's current value is relayed unchanged into its own output slot
(gated on Fresh, so only the measurements THIS event delivered advance), and
the Fresh markers are scrubbed before commit. No Window/Baseline/ZScore/
Velocity stage runs here: the signals already computed their canonical
histories and standardized forms, and a second independent estimator would
silently redefine them.
*/
func latestFactsPipeline(bindings []MetricBinding) nmtypes.Primitive {
	branches := make([]nmtypes.Primitive, 0, len(bindings))

	for _, binding := range bindings {
		branches = append(branches, jointFact(binding))
	}

	return nmtypes.Pipe(nmtypes.ForkStrict(branches...), scrubFresh(bindings))
}

/*
latestFactsOutputs declares one output per bound metric, each borrowing its own
bound metric's Maturity/SNR/SNRDefined and observation-time provenance — the
facts are the signal's own readings, not new derivations.
*/
func latestFactsOutputs(bindings []MetricBinding) []Output {
	outputs := make([]Output, 0, len(bindings))

	for _, binding := range bindings {
		outputs = append(outputs, NewMetricOutput(binding.Series.ValueSymbol, binding))
	}

	return outputs
}

/*
newLatestFactsAdvisor constructs one "latest composed facts" Advisor over a
kind and a set of bindings. All bindings' current values are surfaced as their
own readings in declaration order, with the signal's own provenance.
*/
func newLatestFactsAdvisor(name string, kind types.PerspectiveKind, bindings []MetricBinding) *Advisor {
	return NewAdvisor(name, kind, latestFactsPipeline(bindings), bindings, latestFactsOutputs(bindings))
}
