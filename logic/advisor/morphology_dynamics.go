package advisor

import (
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
MorphologyDynamicsBindings declares the three Morphology change/novelty facts
whose causal historical context the Morphology signal does not yet emit.
Unlike Liquidity, Morphology has no signal-side baseline/z-score/velocity for
these quantities, so the advisor-local temporal context is the one legitimate
place to derive them.
*/
func MorphologyDynamicsBindings() []MetricBinding {
	return []MetricBinding{
		NewMetricBinding("morphology", "morphology_change", "advisor/morphology_dynamics/morphology_change"),
		NewMetricBinding("morphology", "book_shape_distance", "advisor/morphology_dynamics/book_shape_distance"),
		NewMetricBinding("morphology", "book_shape_ks", "advisor/morphology_dynamics/book_shape_ks"),
	}
}

/*
MorphologyDynamicsPipeline composes the temporal context (current value,
adaptive baseline, departure z-score, first difference) of each of the three
bound metrics, each under its own series prefix, gated on Fresh. This is the
one advisor-local temporal pass in this integration; every other advisor whose
signal already publishes canonical histories composes facts directly.
*/
func MorphologyDynamicsPipeline(bindings []MetricBinding) nmtypes.Primitive {
	branches := make([]nmtypes.Primitive, 0, len(bindings))

	for _, binding := range bindings {
		branches = append(branches, freshTemporalContext(binding))
	}

	return nmtypes.Pipe(nmtypes.ForkStrict(branches...), scrubFresh(bindings))
}

/*
MorphologyDynamicsOutputs declares the four named facts per bound metric: its
current value, adaptive baseline, departure z-score, and first difference —
exactly 12 readings.
*/
func MorphologyDynamicsOutputs(bindings []MetricBinding) []Output {
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
NewMorphologyDynamicsAdvisor constructs the KindMorphologyDynamics Advisor: the
causal historical context of Morphology's shape-change facts, derived once
here because the signal does not yet publish them.
*/
func NewMorphologyDynamicsAdvisor(name string) *Advisor {
	bindings := MorphologyDynamicsBindings()

	return NewAdvisor(
		name,
		types.KindMorphologyDynamics,
		MorphologyDynamicsPipeline(bindings),
		bindings,
		MorphologyDynamicsOutputs(bindings),
	)
}
