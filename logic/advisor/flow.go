package advisor

import (
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

// Flow derived-output symbols.
var (
	symbolFlowBookAlignment = nmtypes.MustIntern("advisor/flow/flow_book_alignment")
	symbolFlowBookMaturity  = nmtypes.MustIntern("advisor/flow/flow_book_alignment/maturity")
	symbolFlowBookAtSec     = nmtypes.MustIntern("advisor/flow/flow_book_alignment/at_sec")
	symbolFlowBookAtNsec    = nmtypes.MustIntern("advisor/flow/flow_book_alignment/at_nsec")
)

/*
FlowBindings declares the executed-flow facts (CVD) and displayed-book
structure facts (DepthFlow) the Flow Advisor composes. Every bound metric is
the signal's own reading; only flow_book_alignment is derived by the advisor.
*/
func FlowBindings() []MetricBinding {
	return []MetricBinding{
		NewMetricBinding("cvd", "signed_net_fraction", "advisor/flow/signed_net_fraction"),
		NewMetricBinding("cvd", "signed_net_fraction_zscore", "advisor/flow/signed_net_fraction_zscore"),
		NewMetricBinding("cvd", "net_notional_rate", "advisor/flow/net_notional_rate"),
		NewMetricBinding("cvd", "gross_notional_rate_zscore", "advisor/flow/gross_notional_rate_zscore"),
		NewMetricBinding("cvd", "flow_aligned_midpoint_return", "advisor/flow/flow_aligned_midpoint_return"),
		NewMetricBinding("cvd", "midpoint_response_per_net_notional", "advisor/flow/midpoint_response_per_net_notional"),
		NewMetricBinding("depthflow", "book_imbalance", "advisor/flow/book_imbalance"),
		NewMetricBinding("depthflow", "book_imbalance_zscore", "advisor/flow/book_imbalance_zscore"),
		NewMetricBinding("depthflow", "flow_activity_imbalance", "advisor/flow/flow_activity_imbalance"),
		NewMetricBinding("depthflow", "book_turnover_rate", "advisor/flow/book_turnover_rate"),
	}
}

/*
FlowPipeline relays each bound metric's current value (gated on Fresh) and
computes one derived reading:

  flow_book_alignment = cvd/signed_net_fraction * depthflow/book_imbalance

Both inputs are signed dimensionless quantities. The product's sign states
whether executed-flow direction and displayed-book imbalance agree; its
magnitude retains the joint strength. It is NOT thresholded, NOT turned into
support points, NOT labeled bullish/bearish, and NOT a confidence. It merely
retains the joint alignment, which a consumer reads as context.
*/
func FlowPipeline(bindings []MetricBinding) nmtypes.Primitive {
	signedNet := bindings[0]
	bookImbalance := bindings[6]

	return nmtypes.Pipe(
		nmtypes.ForkStrict(
			jointFact(signedNet),
			jointFact(bindings[1]),
			jointFact(bindings[2]),
			jointFact(bindings[3]),
			jointFact(bindings[4]),
			jointFact(bindings[5]),
			jointFact(bookImbalance),
			jointFact(bindings[7]),
			jointFact(bindings[8]),
			jointFact(bindings[9]),
		),
		flowAlignment(signedNet, bookImbalance),
		scrubFresh(bindings),
	)
}

/*
flowAlignment derives flow_book_alignment from the two retained signed facts,
leaving it undefined until both are present and finite. It never fabricates a
value when an input is absent, and it carries its own maturity (min of the two
parents) and observation instant.
*/
func flowAlignment(signedNet, bookImbalance MetricBinding) nmtypes.Primitive {
	return func(input *nmtypes.Frame) {
		input.Delete(symbolFlowBookAlignment)
		input.Delete(symbolFlowBookMaturity)
		input.Delete(symbolFlowBookAtSec)
		input.Delete(symbolFlowBookAtNsec)

		signedValue, hasSigned := input.Get(signedNet.Series.ValueSymbol)
		bookValue, hasBook := input.Get(bookImbalance.Series.ValueSymbol)

		if !hasSigned || !hasBook {
			return
		}

		result := signedValue * bookValue
		input.Put(symbolFlowBookAlignment, result)

		signedMaturity, _ := input.Get(signedNet.Maturity)
		bookMaturity, _ := input.Get(bookImbalance.Maturity)
		maturity := signedMaturity
		if bookMaturity < maturity {
			maturity = bookMaturity
		}
		input.Put(symbolFlowBookMaturity, maturity)

		if sec, hasSec := input.Get(bookImbalance.Series.SecSymbol); hasSec {
			nsec, _ := input.Get(bookImbalance.Series.NsecSymbol)
			input.Put(symbolFlowBookAtSec, sec)
			input.Put(symbolFlowBookAtNsec, nsec)
		}
	}
}

/*
FlowOutputs declares the ten relayed facts plus the derived flow_book_alignment.
*/
func FlowOutputs(bindings []MetricBinding) []Output {
	outputs := make([]Output, 0, 11)

	for _, binding := range bindings {
		outputs = append(outputs, NewMetricOutput(binding.Series.ValueSymbol, binding))
	}

	outputs = append(outputs, NewDerivedOutputWithTime(
		symbolFlowBookAlignment, symbolFlowBookMaturity,
		bindings[0].FromSec, bindings[0].FromNsec,
		symbolFlowBookAtSec, symbolFlowBookAtNsec,
	))

	return outputs
}

/*
NewFlowAdvisor constructs the KindFlow Advisor: executed flow, price response,
and displayed structure coexist, with flow_book_alignment as the one derived
joint-sign quantity.
*/
func NewFlowAdvisor(name string) *Advisor {
	bindings := FlowBindings()

	return NewAdvisor(name, types.KindFlow, FlowPipeline(bindings), bindings, FlowOutputs(bindings))
}
