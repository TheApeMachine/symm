package advisor

import (
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

// Execution output symbols: the two side-correct flow/share-of-capacity facts
// and the crossing-cost fact. Interned once so bindings/outputs/pipeline all
// read the same derived slots.
var (
	symbolExecutionBuyShare  = nmtypes.MustIntern("advisor/execution/buy_flow_to_ask_capacity")
	symbolExecutionSellShare = nmtypes.MustIntern("advisor/execution/sell_flow_to_bid_capacity")
	symbolExecutionBuyMatur  = nmtypes.MustIntern("advisor/execution/buy_flow_to_ask_capacity/maturity")
	symbolExecutionSellMatur = nmtypes.MustIntern("advisor/execution/sell_flow_to_bid_capacity/maturity")
)

/*
ExecutionBindings declares the executed-flow facts and the displayed-capacity
facts the Execution Context Advisor composes for one symbol:

  - cvd/aggressive_notional:buy — executed aggressive buy notional (quote).
  - cvd/aggressive_notional:sell — executed aggressive sell notional (quote).
  - liquidity/touch_notional:ask — displayed ask-side capacity (quote).
  - liquidity/touch_notional:bid — displayed bid-side capacity (quote).
  - cvd/signed_net_fraction_zscore — flow structure, standardized.
  - liquidity/relative_spread — crossing cost, dimensionless.

Displayed capacity is the exact side-notional the Liquidity spec defines as
"quote-currency notional immediately displayed to market buyers/sellers"
(touch_notional:ask / touch_notional:bid). touch_notional_imbalance is NOT
capacity: it is a dimensionless asymmetry ratio, and a $100/$100 touch and a
$10,000,000/$10,000,000 touch both have imbalance 0 while their actual
capacity differs by five orders of magnitude. Side-correct capacity is the
quantity a flow-vs-capacity reading must use.
*/
func ExecutionBindings() []MetricBinding {
	return []MetricBinding{
		NewMetricBinding("cvd", "aggressive_notional:buy", "advisor/execution/buy_flow"),
		NewMetricBinding("cvd", "aggressive_notional:sell", "advisor/execution/sell_flow"),
		NewMetricBinding("liquidity", "touch_notional:ask", "advisor/execution/ask_capacity"),
		NewMetricBinding("liquidity", "touch_notional:bid", "advisor/execution/bid_capacity"),
		NewMetricBinding("cvd", "signed_net_fraction_zscore", "advisor/execution/flow_structure"),
		NewMetricBinding("liquidity", "relative_spread", "advisor/execution/spread"),
	}
}

/*
ExecutionPipeline relays each bound metric's raw value into its own slot
(gated on Fresh, exactly as liquidity/historical do) and then computes the two
side-correct flow-vs-capacity ratios from whichever values are causally
available in the committed state — regardless of which producer ring delivered
the freshest fact. buy flow is read against ask capacity, sell flow against
bid capacity: an aggressive buy consumes the ask side, never the bid.

The ratios are undefined-safe: a missing or zero capacity, a future-leaked
retained input, or a non-finite quotient leaves the derived slot unset rather
than fabricating a value (see ratioNamed). This is the actual economic
relation the Liquidity §13.1 / CVD §18 decrees, not `flow * confidence` and not
the imbalance asymmetry.
*/
func ExecutionPipeline(bindings []MetricBinding) nmtypes.Primitive {
	branches := make([]nmtypes.Primitive, 0, len(bindings))

	for _, binding := range bindings {
		// The flow-structure and spread facts are descriptive context
		// relayed unchanged; the capacity and flow facts are the ratio inputs.
		branches = append(branches, jointFact(binding))
	}

	buyFlow := bindings[0]
	sellFlow := bindings[1]
	askCap := bindings[2]
	bidCap := bindings[3]

	return nmtypes.Pipe(
		nmtypes.ForkStrict(branches...),
		ratioNamed(
			buyFlow.Series.ValueSymbol, buyFlow.Series.SecSymbol, buyFlow.Series.NsecSymbol,
			askCap.Series.ValueSymbol, askCap.Series.SecSymbol, askCap.Series.NsecSymbol,
			buyFlow.Maturity, askCap.Maturity,
			symbolExecutionBuyShare, symbolExecutionBuyMatur,
		),
		ratioNamed(
			sellFlow.Series.ValueSymbol, sellFlow.Series.SecSymbol, sellFlow.Series.NsecSymbol,
			bidCap.Series.ValueSymbol, bidCap.Series.SecSymbol, bidCap.Series.NsecSymbol,
			sellFlow.Maturity, bidCap.Maturity,
			symbolExecutionSellShare, symbolExecutionSellMatur,
		),
		scrubFresh(bindings),
	)
}

/*
ExecutionOutputs declares the two derived side-correct flow-vs-capacity ratios
plus the underlying measured facts themselves. The raw flow and capacity facts
are exposed with their own observation times (NewMetricOutput), so a consumer
can see that buy flow was observed at t1 while ask capacity was observed at t2;
the derived ratios carry their own min-maturity and no borrowed observation
time. The crossing-cost and flow-structure context facts stay attached for
interpretation.
*/
func ExecutionOutputs(bindings []MetricBinding) []Output {
	return []Output{
		NewDerivedOutput(symbolExecutionBuyShare, symbolExecutionBuyMatur),
		NewDerivedOutput(symbolExecutionSellShare, symbolExecutionSellMatur),
		NewMetricOutput(bindings[0].Series.ValueSymbol, bindings[0]), // aggressive_notional:buy
		NewMetricOutput(bindings[1].Series.ValueSymbol, bindings[1]), // aggressive_notional:sell
		NewMetricOutput(bindings[2].Series.ValueSymbol, bindings[2]), // touch_notional:ask
		NewMetricOutput(bindings[3].Series.ValueSymbol, bindings[3]), // touch_notional:bid
		NewMetricOutput(bindings[4].Series.ValueSymbol, bindings[4]), // signed_net_fraction_zscore
		NewMetricOutput(bindings[5].Series.ValueSymbol, bindings[5]), // relative_spread
	}
}

/*
NewExecutionAdvisor constructs the single concrete execution-context Advisor
instance over ExecutionPipeline and ExecutionBindings. It answers the named
question: how much of the displayed ask/bid capacity did the current aggressive
buy/sell flow represent — side-correct, never an imbalance asymmetry.
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
