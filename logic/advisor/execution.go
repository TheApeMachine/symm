package advisor

import (
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

// Execution output symbols: the two side-correct executed-flow-per-displayed-
// capacity facts and their per-reading temporal provenance slots.
var (
	symbolExecutionBuyCoverage  = nmtypes.MustIntern("advisor/execution/buy_flow_per_ask_touch")
	symbolExecutionSellCoverage = nmtypes.MustIntern("advisor/execution/sell_flow_per_bid_touch")
	symbolExecutionBuyMatur     = nmtypes.MustIntern("advisor/execution/buy_flow_per_ask_touch/maturity")
	symbolExecutionSellMatur    = nmtypes.MustIntern("advisor/execution/sell_flow_per_bid_touch/maturity")
	symbolExecutionBuyFromSec   = nmtypes.MustIntern("advisor/execution/buy_flow_per_ask_touch/from_sec")
	symbolExecutionBuyFromNsec  = nmtypes.MustIntern("advisor/execution/buy_flow_per_ask_touch/from_nsec")
	symbolExecutionBuyAtSec     = nmtypes.MustIntern("advisor/execution/buy_flow_per_ask_touch/at_sec")
	symbolExecutionBuyAtNsec    = nmtypes.MustIntern("advisor/execution/buy_flow_per_ask_touch/at_nsec")
	symbolExecutionSellFromSec  = nmtypes.MustIntern("advisor/execution/sell_flow_per_bid_touch/from_sec")
	symbolExecutionSellFromNsec = nmtypes.MustIntern("advisor/execution/sell_flow_per_bid_touch/from_nsec")
	symbolExecutionSellAtSec    = nmtypes.MustIntern("advisor/execution/sell_flow_per_bid_touch/at_sec")
	symbolExecutionSellAtNsec   = nmtypes.MustIntern("advisor/execution/sell_flow_per_bid_touch/at_nsec")
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
capacity differs by five orders of magnitude.

TEMPORAL COORDINATE. The two ratio inputs have deliberately different scopes,
and the derived quantity is defined in terms of them rather than papering the
difference over:

  - cvd/aggressive_notional:buy/sell is an INTERVAL quantity: the accumulated
    aggressive notional over the CVD signal's retained path [From, At], where
    From is the first retained trade and At the last. Its TimescaleInstantaneous
    label marks that it is a state value at At, not that it is a point
    measurement.
  - liquidity/touch_notional:ask/bid is a POINT-IN-TIME displayed quantity: the
    book state at the liquidity snapshot.

Therefore the derived ratio is NOT "the fraction of current displayed capacity
that current flow consumed." It is the CVD §18 / Liquidity §13.1 coverage
factor: executed aggressive notional over the retained interval, relative to
the latest causally-available displayed touch on the aggressive side. A longer
retained flow interval increases the numerator without changing the book, so
the ratio reports coverage-over-interval, never instantaneous consumption. The
reading carries its own From and ObservedAt so a consumer can see exactly which
interval the numerator spans and which snapshot the denominator reflects.
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
(gated on Fresh) and computes the two side-correct coverage factors from the
causally available committed state — regardless of which producer ring delivered
the freshest fact. buy flow is read against ask capacity, sell flow against
bid capacity: an aggressive buy consumes the ask side, never the bid.

The coverage factor is future-leak-rejected but explicitly interval-aware: it
uses the interval flow (numerator) against the point capacity (denominator), as
defined in the ExecutionBindings temporal coordinate. It is named per-ask-touch
and per-bid-touch — a coverage ratio with unit quote/quote, dimensionless —
never a claim that current flow "filled" current capacity.
*/
func ExecutionPipeline(bindings []MetricBinding) nmtypes.Primitive {
	branches := make([]nmtypes.Primitive, 0, len(bindings))

	for _, binding := range bindings {
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
			buyFlow.FromSec, buyFlow.FromNsec,
			askCap.Series.ValueSymbol, askCap.Series.SecSymbol, askCap.Series.NsecSymbol,
			buyFlow.Maturity, askCap.Maturity,
			symbolExecutionBuyCoverage, symbolExecutionBuyMatur,
			symbolExecutionBuyFromSec, symbolExecutionBuyFromNsec,
			symbolExecutionBuyAtSec, symbolExecutionBuyAtNsec,
		),
		ratioNamed(
			sellFlow.Series.ValueSymbol, sellFlow.Series.SecSymbol, sellFlow.Series.NsecSymbol,
			sellFlow.FromSec, sellFlow.FromNsec,
			bidCap.Series.ValueSymbol, bidCap.Series.SecSymbol, bidCap.Series.NsecSymbol,
			sellFlow.Maturity, bidCap.Maturity,
			symbolExecutionSellCoverage, symbolExecutionSellMatur,
			symbolExecutionSellFromSec, symbolExecutionSellFromNsec,
			symbolExecutionSellAtSec, symbolExecutionSellAtNsec,
		),
		scrubFresh(bindings),
	)
}

/*
ExecutionOutputs declares the two side-correct coverage factors plus the
underlying measured facts themselves. The raw flow and capacity facts are
exposed with their own observation times (and From intervals), so a consumer
can see that buy flow spanned [From, At] while ask capacity is the point
snapshot at ObservedAt; the derived coverage factors carry their own min-
maturity and no borrowed observation time.
*/
func ExecutionOutputs(bindings []MetricBinding) []Output {
	return []Output{
		NewDerivedOutputWithTime(
			symbolExecutionBuyCoverage, symbolExecutionBuyMatur,
			symbolExecutionBuyFromSec, symbolExecutionBuyFromNsec,
			symbolExecutionBuyAtSec, symbolExecutionBuyAtNsec,
		),
		NewDerivedOutputWithTime(
			symbolExecutionSellCoverage, symbolExecutionSellMatur,
			symbolExecutionSellFromSec, symbolExecutionSellFromNsec,
			symbolExecutionSellAtSec, symbolExecutionSellAtNsec,
		),
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
question: how much executed aggressive flow (over its retained interval) sits
relative to the side of the book it executed into — side-correct, never an
imbalance asymmetry, and with the interval-vs-point temporal coordinate stated
explicitly in the binding.
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
