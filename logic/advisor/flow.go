package advisor

import (
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

// Flow derived-output symbols.
var (
	symbolFlowMutationAlignment = nmtypes.MustIntern("advisor/flow/flow_mutation_alignment")
	symbolFlowMutationMaturity  = nmtypes.MustIntern("advisor/flow/flow_mutation_alignment/maturity")
	symbolFlowMutationAtSec     = nmtypes.MustIntern("advisor/flow/flow_mutation_alignment/at_sec")
	symbolFlowMutationAtNsec    = nmtypes.MustIntern("advisor/flow/flow_mutation_alignment/at_nsec")
)

/*
FlowBindings declares the executed-flow facts (CVD) and Level-3 mutation facts
(DepthFlow) the Flow Advisor composes. Every bound metric is the signal's own
reading; only flow_mutation_alignment is derived by the advisor.
*/
func FlowBindings() []MetricBinding {
	return []MetricBinding{
		NewMetricBinding("cvd", "signed_net_fraction", "advisor/flow/signed_net_fraction"),
		NewMetricBinding("cvd", "signed_net_fraction_zscore", "advisor/flow/signed_net_fraction_zscore"),
		NewMetricBinding("cvd", "net_notional_rate", "advisor/flow/net_notional_rate"),
		NewMetricBinding("cvd", "gross_notional_rate_zscore", "advisor/flow/gross_notional_rate_zscore"),
		NewMetricBinding("cvd", "flow_aligned_midpoint_return", "advisor/flow/flow_aligned_midpoint_return"),
		NewMetricBinding("cvd", "midpoint_response_per_net_notional", "advisor/flow/midpoint_response_per_net_notional"),
		NewMetricBinding("depthflow", "observed_notional_imbalance", "advisor/flow/observed_notional_imbalance"),
		NewMetricBinding("depthflow", "observed_notional_imbalance_zscore", "advisor/flow/observed_notional_imbalance_zscore"),
		NewMetricBinding("depthflow", "mutation_activity_imbalance", "advisor/flow/mutation_activity_imbalance"),
		NewMetricBinding("depthflow", "observed_notional_rate", "advisor/flow/observed_notional_rate"),
	}
}

/*
FlowPipeline relays each bound metric's current value (gated on Fresh) and
computes one derived reading:

	flow_mutation_alignment = cvd/signed_net_fraction * depthflow/observed_notional_imbalance

Both inputs are signed dimensionless quantities. The product's sign states
whether executed-flow direction and displayed-book imbalance agree; its
magnitude retains the joint strength. It is NOT thresholded, NOT turned into
support points, NOT labeled bullish/bearish, and NOT a confidence. It merely
retains the joint alignment, which a consumer reads as context.
*/
func FlowPipeline(bindings []MetricBinding) nmtypes.Primitive {
	signedNet := bindings[0]
	mutationImbalance := bindings[6]

	return nmtypes.Pipe(
		nmtypes.ForkStrict(
			jointFact(signedNet),
			jointFact(bindings[1]),
			jointFact(bindings[2]),
			jointFact(bindings[3]),
			jointFact(bindings[4]),
			jointFact(bindings[5]),
			jointFact(mutationImbalance),
			jointFact(bindings[7]),
			jointFact(bindings[8]),
			jointFact(bindings[9]),
		),
		flowAlignment(signedNet, mutationImbalance),
		scrubFresh(bindings),
	)
}

/*
flowAlignment derives flow_mutation_alignment from the retained signed facts,
leaving it undefined until both are present and finite. It never fabricates a
value when an input is absent, and it carries its own maturity (min of the two
parents) and observation instant.
*/
func flowAlignment(signedNet, mutationImbalance MetricBinding) nmtypes.Primitive {
	return func(input *nmtypes.Frame) {
		input.Delete(symbolFlowMutationAlignment)
		input.Delete(symbolFlowMutationMaturity)
		input.Delete(symbolFlowMutationAtSec)
		input.Delete(symbolFlowMutationAtNsec)

		signedValue, hasSigned := input.Get(signedNet.Series.ValueSymbol)
		mutationValue, hasMutation := input.Get(mutationImbalance.Series.ValueSymbol)

		if !hasSigned || !hasMutation {
			return
		}

		result := signedValue * mutationValue
		input.Put(symbolFlowMutationAlignment, result)

		signedMaturity, _ := input.Get(signedNet.Maturity)
		mutationMaturity, _ := input.Get(mutationImbalance.Maturity)
		maturity := signedMaturity

		if mutationMaturity < maturity {
			maturity = mutationMaturity
		}
		input.Put(symbolFlowMutationMaturity, maturity)

		if sec, hasSec := input.Get(mutationImbalance.Series.SecSymbol); hasSec {
			nsec, _ := input.Get(mutationImbalance.Series.NsecSymbol)
			input.Put(symbolFlowMutationAtSec, sec)
			input.Put(symbolFlowMutationAtNsec, nsec)
		}
	}
}

/*
FlowOutputs declares the ten relayed facts plus flow_mutation_alignment.
*/
func FlowOutputs(bindings []MetricBinding) []Output {
	outputs := make([]Output, 0, 11)

	for _, binding := range bindings {
		outputs = append(outputs, NewMetricOutput(binding.Series.ValueSymbol, binding))
	}

	outputs = append(outputs, NewDerivedOutputWithTime(
		symbolFlowMutationAlignment, symbolFlowMutationMaturity,
		bindings[0].FromSec, bindings[0].FromNsec,
		symbolFlowMutationAtSec, symbolFlowMutationAtNsec,
	))

	return outputs
}

/*
NewFlowAdvisor constructs the KindFlow Advisor: executed flow, price response,
and mutation structure coexist, with flow_mutation_alignment as the one derived
joint-sign quantity.
*/
func NewFlowAdvisor(name string) *Advisor {
	bindings := FlowBindings()

	return NewAdvisor(name, types.KindFlow, FlowPipeline(bindings), bindings, FlowOutputs(bindings))
}
