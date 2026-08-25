package cvd

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/kraken/websocket"
)

/*
Estimator series prefixes: one independent namespaced estimator per measured
quantity, so a single frame carries several adaptive baselines and velocities
without colliding on the legacy generic slots.
*/
const (
	prefixGrossRate = "cvd/gross_rate"
	prefixNetRate   = "cvd/net_rate"
	prefixNetFrac   = "cvd/net_fraction"
	prefixMidRate   = "cvd/midpoint_rate"
)

/*
Input, derived, and state slot symbols for the executed-flow pipeline.
*/
var (
	// Loaded trade facts.
	symbolPrice = nmtypes.MustIntern("cvd/price")
	symbolQty   = nmtypes.MustIntern("cvd/qty")
	symbolSign  = nmtypes.MustIntern("cvd/sign")

	// Per-step derived facts.
	symbolNotional          = nmtypes.MustIntern("cvd/notional")
	symbolSignedQty         = nmtypes.MustIntern("cvd/signed_qty")
	symbolSignedNotional    = nmtypes.MustIntern("cvd/signed_notional")
	symbolNegSignedQty      = nmtypes.MustIntern("cvd/neg_signed_qty")
	symbolNegSignedNotional = nmtypes.MustIntern("cvd/neg_signed_notional")
	symbolNegSign           = nmtypes.MustIntern("cvd/neg_sign")
	symbolBuyQty            = nmtypes.MustIntern("cvd/buy_qty")
	symbolSellQty           = nmtypes.MustIntern("cvd/sell_qty")
	symbolBuyNotional       = nmtypes.MustIntern("cvd/buy_notional")
	symbolSellNotional      = nmtypes.MustIntern("cvd/sell_notional")
	symbolBuyCountStep      = nmtypes.MustIntern("cvd/buy_count_step")
	symbolSellCountStep     = nmtypes.MustIntern("cvd/sell_count_step")
	symbolUnit              = nmtypes.MustIntern("cvd/unit")

	// Stateful totals.
	symbolBuyQtyTotal       = nmtypes.MustIntern("cvd/state/buy_qty_total")
	symbolSellQtyTotal      = nmtypes.MustIntern("cvd/state/sell_qty_total")
	symbolBuyNotionalTotal  = nmtypes.MustIntern("cvd/state/buy_notional_total")
	symbolSellNotionalTotal = nmtypes.MustIntern("cvd/state/sell_notional_total")
	symbolBuyCountTotal     = nmtypes.MustIntern("cvd/state/buy_count_total")
	symbolSellCountTotal    = nmtypes.MustIntern("cvd/state/sell_count_total")

	// Aggregates over the retained trade path.
	symbolGrossQty            = nmtypes.MustIntern("cvd/gross_executed_quantity")
	symbolNetQty              = nmtypes.MustIntern("cvd/net_executed_quantity")
	symbolGrossNotional       = nmtypes.MustIntern("cvd/gross_notional")
	symbolNetNotional         = nmtypes.MustIntern("cvd/net_notional")
	symbolSignedCount         = nmtypes.MustIntern("cvd/signed_count")
	symbolSignedCountFraction = nmtypes.MustIntern("cvd/signed_count_fraction")
	symbolNetFraction         = nmtypes.MustIntern("cvd/signed_net_fraction")
	symbolMeanNotional        = nmtypes.MustIntern("cvd/mean_trade_notional")

	// Execution-rate facts.
	symbolGrossRate     = nmtypes.MustIntern("cvd/gross_notional_rate")
	symbolNetRate       = nmtypes.MustIntern("cvd/net_notional_rate")
	symbolBuyRate       = nmtypes.MustIntern("cvd/buy_notional_rate")
	symbolSellRate      = nmtypes.MustIntern("cvd/sell_notional_rate")
	symbolLogGross      = nmtypes.MustIntern("cvd/log_gross_notional_rate")
	symbolGrossBase     = nmtypes.MustIntern("cvd/gross_notional_rate_baseline")
	symbolGrossRatio    = nmtypes.MustIntern("cvd/gross_notional_rate_ratio")
	symbolNetVelocity   = nmtypes.MustIntern("cvd/net_notional_rate_velocity")
	symbolGrossVelocity = nmtypes.MustIntern("cvd/gross_notional_rate_velocity")

	// Response-price facts loaded from the shared book.
	symbolBidPrice        = nmtypes.MustIntern("cvd/bid_price")
	symbolAskPrice        = nmtypes.MustIntern("cvd/ask_price")
	symbolBidFrom         = nmtypes.MustIntern("cvd/state/bid_from")
	symbolAskFrom         = nmtypes.MustIntern("cvd/state/ask_from")
	symbolHasQuote        = nmtypes.MustIntern("cvd/has_quote")
	symbolMidpoint        = nmtypes.MustIntern("cvd/response_midpoint_at")
	symbolMidpointFrom    = nmtypes.MustIntern("cvd/response_midpoint_from")
	symbolMidpointLog     = nmtypes.MustIntern("cvd/midpoint_log_return")
	symbolMidpointRate    = nmtypes.MustIntern("cvd/midpoint_return_rate")
	symbolAbsNetNotional  = nmtypes.MustIntern("cvd/abs_net_notional")
	symbolSignNetNotional = nmtypes.MustIntern("cvd/sign_net_notional")
	symbolFlowAligned     = nmtypes.MustIntern("cvd/flow_aligned_midpoint_return")
	symbolResponsePerNet  = nmtypes.MustIntern("cvd/midpoint_response_per_net_notional")

	// Validation coordinates.
	symbolZero          = nmtypes.MustIntern("cvd/zero")
	symbolOne           = nmtypes.MustIntern("cvd/one")
	symbolInvalid       = nmtypes.MustIntern("cvd/invalid_input")
	symbolPricePositive = nmtypes.MustIntern("cvd/price_positive")
	symbolQtyPositive   = nmtypes.MustIntern("cvd/qty_positive")
)

/*
prefixed resolves one namespaced estimator slot for a series prefix.
*/
func prefixed(prefix string, name string) nmtypes.Symbol {
	return nmtypes.MustIntern(temporal.JoinPrefix(prefix, name))
}

/*
preBaselineSlot names the pre-observation baseline fact for a series prefix.
*/
func preBaselineSlot(prefix string) nmtypes.Symbol {
	return prefixed(prefix, "baseline/previous")
}

/*
adaptiveEstimator composes one causal adaptive baseline with its z-score. The
quantity and event clock are fed into the series, then ZScore reads the previous
committed baseline (global spec §4 causality) before the pre-observation
baseline is captured, Baseline adapts, and Window retains the observation.
*/
func adaptiveEstimator(prefix string, quantity nmtypes.Symbol) nmtypes.Primitive {
	series := temporal.NewSeries(prefix)
	residual := prefixed(prefix, "z/residual")

	return nmtypes.Pipe(
		nmtypes.Relay(quantity, series.ValueSymbol),
		nmtypes.Relay(nmtypes.EventTimeSec, series.SecSymbol),
		nmtypes.Relay(nmtypes.EventTimeNsec, series.NsecSymbol),
		statistic.ZScore(prefix),
		// Pre-observation baseline: value - residual = previous committed
		// baseline, captured before Baseline updates it with the observation.
		logic.If(
			nmtypes.Wire(
				nmtypes.Identity,
				nmtypes.In(series.ReadySymbol, nmtypes.PortX),
				nmtypes.Out(nmtypes.PortX, logic.SymbolCondition),
			),
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(quantity, calculus.PortA),
				nmtypes.In(residual, calculus.PortB),
				nmtypes.Out(calculus.PortResult, preBaselineSlot(prefix)),
			),
			nmtypes.Identity,
		),
		statistic.Baseline(prefix),
		temporal.Window(prefix),
	)
}

/*
velocityEstimator composes one causal first-difference velocity over a series.
*/
func velocityEstimator(prefix string, quantity nmtypes.Symbol) nmtypes.Primitive {
	series := temporal.NewSeries(prefix)

	return nmtypes.Pipe(
		nmtypes.Relay(quantity, series.ValueSymbol),
		nmtypes.Relay(nmtypes.EventTimeSec, series.SecSymbol),
		nmtypes.Relay(nmtypes.EventTimeNsec, series.NsecSymbol),
		statistic.Velocity(prefix),
	)
}

/*
velocityRate projects a series' first difference into its per-second rate,
gated on the differencer having produced a delta.
*/
func velocityRate(prefix string, target nmtypes.Symbol) nmtypes.Primitive {
	delta := prefixed(prefix, "velocity/delta")
	elapsed := prefixed(prefix, "velocity/elapsed_sec")
	ready := prefixed(prefix, "ready")

	return logic.If(
		nmtypes.Wire(
			nmtypes.Identity,
			nmtypes.In(ready, nmtypes.PortX),
			nmtypes.Out(nmtypes.PortX, logic.SymbolCondition),
		),
		nmtypes.Wire(
			calculus.Quotient,
			nmtypes.In(delta, calculus.PortA),
			nmtypes.In(elapsed, calculus.PortB),
			nmtypes.Out(calculus.PortResult, target),
		),
		nmtypes.Identity,
	)
}

/*
cvdPipeline composes the executed-flow reducer. Direct accounting (counts,
quantities, notionals, fractions, cumulative deltas) is always emitted; rates,
velocities, and response-price metrics are gated on their own prerequisites.
*/
func cvdPipeline() nmtypes.Primitive {
	return nmtypes.Pipe(
		// Reject non-finite input.
		logic.EnsureFinite(
			symbolPrice,
			symbolQty,
			symbolSign,
			nmtypes.EventTimeSec,
			nmtypes.EventTimeNsec,
		),

		// Reject non-positive execution price or quantity.
		nmtypes.Assign(symbolZero, 0),
		nmtypes.Assign(symbolOne, 1),
		logic.If(
			nmtypes.Pipe(
				nmtypes.Wire(
					logic.GreaterThan,
					nmtypes.In(symbolPrice, calculus.PortA),
					nmtypes.In(symbolZero, calculus.PortB),
					nmtypes.Out(logic.SymbolCondition, symbolPricePositive),
				),
				nmtypes.Wire(
					logic.GreaterThan,
					nmtypes.In(symbolQty, calculus.PortA),
					nmtypes.In(symbolZero, calculus.PortB),
					nmtypes.Out(logic.SymbolCondition, symbolQtyPositive),
				),
				nmtypes.Wire(
					logic.And,
					nmtypes.In(symbolPricePositive, calculus.PortA),
					nmtypes.In(symbolQtyPositive, calculus.PortB),
					nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
				),
			),
			nmtypes.Identity,
			logic.Observe(symbolInvalid),
		),

		// Executed notional and signed flow.
		nmtypes.Wire(
			calculus.Product,
			nmtypes.In(symbolPrice, calculus.PortA),
			nmtypes.In(symbolQty, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolNotional),
		),
		nmtypes.Wire(
			calculus.Product,
			nmtypes.In(symbolSign, calculus.PortA),
			nmtypes.In(symbolQty, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolSignedQty),
		),
		nmtypes.Wire(
			calculus.Product,
			nmtypes.In(symbolSign, calculus.PortA),
			nmtypes.In(symbolNotional, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolSignedNotional),
		),

		// Split signed flow into buy and sell contributions.
		nmtypes.Wire(
			calculus.Positive,
			nmtypes.In(symbolSignedQty, calculus.PortX),
			nmtypes.Out(calculus.PortResult, symbolBuyQty),
		),
		nmtypes.Wire(
			calculus.Negative,
			nmtypes.In(symbolSignedQty, calculus.PortX),
			nmtypes.Out(calculus.PortResult, symbolNegSignedQty),
		),
		nmtypes.Wire(
			calculus.Positive,
			nmtypes.In(symbolNegSignedQty, calculus.PortX),
			nmtypes.Out(calculus.PortResult, symbolSellQty),
		),
		nmtypes.Wire(
			calculus.Positive,
			nmtypes.In(symbolSignedNotional, calculus.PortX),
			nmtypes.Out(calculus.PortResult, symbolBuyNotional),
		),
		nmtypes.Wire(
			calculus.Negative,
			nmtypes.In(symbolSignedNotional, calculus.PortX),
			nmtypes.Out(calculus.PortResult, symbolNegSignedNotional),
		),
		nmtypes.Wire(
			calculus.Positive,
			nmtypes.In(symbolNegSignedNotional, calculus.PortX),
			nmtypes.Out(calculus.PortResult, symbolSellNotional),
		),

		// Aggressor-side count indicators and the constant unit event.
		nmtypes.Wire(
			calculus.Positive,
			nmtypes.In(symbolSign, calculus.PortX),
			nmtypes.Out(calculus.PortResult, symbolBuyCountStep),
		),
		nmtypes.Wire(
			calculus.Negative,
			nmtypes.In(symbolSign, calculus.PortX),
			nmtypes.Out(calculus.PortResult, symbolNegSign),
		),
		nmtypes.Wire(
			calculus.Positive,
			nmtypes.In(symbolNegSign, calculus.PortX),
			nmtypes.Out(calculus.PortResult, symbolSellCountStep),
		),
		nmtypes.Wire(
			calculus.Absolute,
			nmtypes.In(symbolSign, calculus.PortX),
			nmtypes.Out(calculus.PortResult, symbolUnit),
		),

		// Accumulate quantity, notional, and count totals.
		nmtypes.Wire(
			calculus.Accumulate,
			nmtypes.In(symbolBuyQty, calculus.SymbolDelta),
			nmtypes.State(symbolBuyQtyTotal, calculus.SymbolTotal),
		),
		nmtypes.Wire(
			calculus.Accumulate,
			nmtypes.In(symbolSellQty, calculus.SymbolDelta),
			nmtypes.State(symbolSellQtyTotal, calculus.SymbolTotal),
		),
		nmtypes.Wire(
			calculus.Accumulate,
			nmtypes.In(symbolBuyNotional, calculus.SymbolDelta),
			nmtypes.State(symbolBuyNotionalTotal, calculus.SymbolTotal),
		),
		nmtypes.Wire(
			calculus.Accumulate,
			nmtypes.In(symbolSellNotional, calculus.SymbolDelta),
			nmtypes.State(symbolSellNotionalTotal, calculus.SymbolTotal),
		),
		nmtypes.Wire(
			calculus.Accumulate,
			nmtypes.In(symbolBuyCountStep, calculus.SymbolDelta),
			nmtypes.State(symbolBuyCountTotal, calculus.SymbolTotal),
		),
		nmtypes.Wire(
			calculus.Accumulate,
			nmtypes.In(symbolSellCountStep, calculus.SymbolDelta),
			nmtypes.State(symbolSellCountTotal, calculus.SymbolTotal),
		),

		// Trade count N (also the measurement support count).
		nmtypes.Wire(
			calculus.Sum,
			nmtypes.In(symbolBuyCountTotal, calculus.PortA),
			nmtypes.In(symbolSellCountTotal, calculus.PortB),
			nmtypes.Out(calculus.PortResult, nmtypes.SampleCount),
		),

		// Quantity and notional aggregates.
		nmtypes.Wire(
			calculus.Sum,
			nmtypes.In(symbolBuyQtyTotal, calculus.PortA),
			nmtypes.In(symbolSellQtyTotal, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolGrossQty),
		),
		nmtypes.Wire(
			calculus.Difference,
			nmtypes.In(symbolBuyQtyTotal, calculus.PortA),
			nmtypes.In(symbolSellQtyTotal, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolNetQty),
		),
		nmtypes.Wire(
			calculus.Sum,
			nmtypes.In(symbolBuyNotionalTotal, calculus.PortA),
			nmtypes.In(symbolSellNotionalTotal, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolGrossNotional),
		),
		nmtypes.Wire(
			calculus.Difference,
			nmtypes.In(symbolBuyNotionalTotal, calculus.PortA),
			nmtypes.In(symbolSellNotionalTotal, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolNetNotional),
		),
		nmtypes.Wire(
			calculus.Difference,
			nmtypes.In(symbolBuyCountTotal, calculus.PortA),
			nmtypes.In(symbolSellCountTotal, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolSignedCount),
		),

		// Scale-free fractions and mean size.
		nmtypes.Wire(
			calculus.Quotient,
			nmtypes.In(symbolSignedCount, calculus.PortA),
			nmtypes.In(nmtypes.SampleCount, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolSignedCountFraction),
		),
		nmtypes.Wire(
			calculus.Quotient,
			nmtypes.In(symbolNetNotional, calculus.PortA),
			nmtypes.In(symbolGrossNotional, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolNetFraction),
		),
		nmtypes.Wire(
			calculus.Quotient,
			nmtypes.In(symbolGrossNotional, calculus.PortA),
			nmtypes.In(nmtypes.SampleCount, calculus.PortB),
			nmtypes.Out(calculus.PortResult, symbolMeanNotional),
		),

		// Retained-path origin and elapsed duration.
		temporal.Since,

		// Directional-flow baseline and z-score (additive, bounded quantity).
		adaptiveEstimator(prefixNetFrac, symbolNetFraction),

		// Execution rates, velocities, and response-price metrics, defined only
		// once a positive interval elapsed.
		logic.If(
			nmtypes.Wire(
				nmtypes.Identity,
				nmtypes.In(temporal.SymbolAdvanced, nmtypes.PortX),
				nmtypes.Out(nmtypes.PortX, logic.SymbolCondition),
			),
			nmtypes.Pipe(
				calculus.Rate,
				nmtypes.Wire(
					calculus.Quotient,
					nmtypes.In(symbolGrossNotional, calculus.PortA),
					nmtypes.In(calculus.SymbolDuration, calculus.PortB),
					nmtypes.Out(calculus.PortResult, symbolGrossRate),
				),
				nmtypes.Wire(
					calculus.Quotient,
					nmtypes.In(symbolNetNotional, calculus.PortA),
					nmtypes.In(calculus.SymbolDuration, calculus.PortB),
					nmtypes.Out(calculus.PortResult, symbolNetRate),
				),
				nmtypes.Wire(
					calculus.Quotient,
					nmtypes.In(symbolBuyNotionalTotal, calculus.PortA),
					nmtypes.In(calculus.SymbolDuration, calculus.PortB),
					nmtypes.Out(calculus.PortResult, symbolBuyRate),
				),
				nmtypes.Wire(
					calculus.Quotient,
					nmtypes.In(symbolSellNotionalTotal, calculus.PortA),
					nmtypes.In(calculus.SymbolDuration, calculus.PortB),
					nmtypes.Out(calculus.PortResult, symbolSellRate),
				),

				// Gross-flow baseline in log space (positive multiplicative).
				nmtypes.Wire(
					calculus.Log,
					nmtypes.In(symbolGrossRate, calculus.PortX),
					nmtypes.Out(calculus.PortResult, symbolLogGross),
				),
				adaptiveEstimator(prefixGrossRate, symbolLogGross),

				// Gross-flow baseline and ratio, defined once the baseline has
				// retained more than one observation.
				logic.If(
					nmtypes.Wire(
						logic.GreaterThan,
						nmtypes.In(prefixed(prefixGrossRate, "count"), calculus.PortA),
						nmtypes.In(symbolOne, calculus.PortB),
						nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
					),
					nmtypes.Pipe(
						nmtypes.Wire(
							calculus.Exp,
							nmtypes.In(preBaselineSlot(prefixGrossRate), calculus.PortX),
							nmtypes.Out(calculus.PortResult, symbolGrossBase),
						),
						nmtypes.Wire(
							calculus.Exp,
							nmtypes.In(prefixed(prefixGrossRate, "z/residual"), calculus.PortX),
							nmtypes.Out(calculus.PortResult, symbolGrossRatio),
						),
					),
					nmtypes.Identity,
				),

				// Signed-flow rate velocity and gross-rate velocity.
				velocityEstimator(prefixNetRate, symbolNetRate),
				velocityRate(prefixNetRate, symbolNetVelocity),
				velocityEstimator(prefixGrossRate+"_vel", symbolLogGross),
				velocityRate(prefixGrossRate+"_vel", symbolGrossVelocity),

				// Response-price metrics, defined only with a valid quote.
				logic.If(
					nmtypes.Wire(
						nmtypes.Identity,
						nmtypes.In(symbolHasQuote, nmtypes.PortX),
						nmtypes.Out(nmtypes.PortX, logic.SymbolCondition),
					),
					nmtypes.Pipe(
						nmtypes.Wire(
							calculus.Average,
							nmtypes.In(symbolBidPrice, calculus.PortA),
							nmtypes.In(symbolAskPrice, calculus.PortB),
							nmtypes.Out(calculus.PortResult, symbolMidpoint),
						),
						nmtypes.Wire(
							calculus.Average,
							nmtypes.In(symbolBidFrom, calculus.PortA),
							nmtypes.In(symbolAskFrom, calculus.PortB),
							nmtypes.Out(calculus.PortResult, symbolMidpointFrom),
						),
						nmtypes.Wire(
							calculus.LogRatio,
							nmtypes.In(symbolMidpoint, calculus.SymbolCurrent),
							nmtypes.In(symbolMidpointFrom, calculus.SymbolPrevious),
							nmtypes.Out(calculus.PortResult, symbolMidpointLog),
						),
						nmtypes.Wire(
							calculus.Quotient,
							nmtypes.In(symbolMidpointLog, calculus.PortA),
							nmtypes.In(calculus.SymbolDuration, calculus.PortB),
							nmtypes.Out(calculus.PortResult, symbolMidpointRate),
						),
						adaptiveEstimator(prefixMidRate, symbolMidpointRate),

						// Flow-aligned return and response per net notional,
						// undefined when net notional is zero.
						nmtypes.Wire(
							calculus.Absolute,
							nmtypes.In(symbolNetNotional, calculus.PortX),
							nmtypes.Out(calculus.PortResult, symbolAbsNetNotional),
						),
						logic.If(
							nmtypes.Wire(
								logic.GreaterThan,
								nmtypes.In(symbolAbsNetNotional, calculus.PortA),
								nmtypes.In(symbolZero, calculus.PortB),
								nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
							),
							nmtypes.Pipe(
								nmtypes.Wire(
									calculus.Quotient,
									nmtypes.In(symbolNetNotional, calculus.PortA),
									nmtypes.In(symbolAbsNetNotional, calculus.PortB),
									nmtypes.Out(calculus.PortResult, symbolSignNetNotional),
								),
								nmtypes.Wire(
									calculus.Product,
									nmtypes.In(symbolSignNetNotional, calculus.PortA),
									nmtypes.In(symbolMidpointLog, calculus.PortB),
									nmtypes.Out(calculus.PortResult, symbolFlowAligned),
								),
								nmtypes.Wire(
									calculus.Quotient,
									nmtypes.In(symbolMidpointLog, calculus.PortA),
									nmtypes.In(symbolNetNotional, calculus.PortB),
									nmtypes.Out(calculus.PortResult, symbolResponsePerNet),
								),
							),
							nmtypes.Identity,
						),
					),
					nmtypes.Identity,
				),
			),
			nmtypes.Identity,
		),
	)
}

/*
Trade is the executed-flow market entity. It owns exactly a Number pipeline
and a projector, both declared in its constructor, plus Step and Close. The
response-price metrics read the shared book from the workspace pool; without a
valid quote they are simply undefined rather than fabricated.

The optional causal flow-response regression (README §15: flow_response_intercept,
flow_response_coefficient, expected_midpoint_return_rate, flow_response_residual,
flow_response_residual_snr) is not emitted: it requires a linear-regression
estimator, and nomagique exposes no such types.Primitive. The README marks those
metrics MAY-optional ("emitted only when the regression is estimable").
*/
type Trade struct {
	workspace *runtime.Workspace
	number    *nomagique.Number[string]
	projector *data.Projector
}

/*
NewTrade constructs the Trade entity: one Number pipeline for executed-flow
accounting and one projector that names the output slots.
*/
func NewTrade(workspace *runtime.Workspace) *Trade {
	return &Trade{
		workspace: workspace,
		number:    nomagique.NewNumber[string](cvdPipeline()),
		projector: data.NewProjector(
			data.Binding{From: nmtypes.SampleCount, Name: "trade_count", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBuyCountTotal, Name: "trade_count:buy", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolSellCountTotal, Name: "trade_count:sell", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolSignedCountFraction, Name: "signed_count_fraction", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBuyQtyTotal, Name: "executed_quantity:buy", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolSellQtyTotal, Name: "executed_quantity:sell", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolGrossQty, Name: "gross_executed_quantity", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolNetQty, Name: "net_executed_quantity", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBuyNotionalTotal, Name: "aggressive_notional:buy", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolSellNotionalTotal, Name: "aggressive_notional:sell", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolGrossNotional, Name: "gross_notional", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolNetNotional, Name: "net_notional", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolNetFraction, Name: "signed_net_fraction", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolMeanNotional, Name: "mean_trade_notional", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: calculus.SymbolRate, Name: "trade_rate", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolGrossRate, Name: "gross_notional_rate", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolNetRate, Name: "net_notional_rate", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolBuyRate, Name: "buy_notional_rate", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolSellRate, Name: "sell_notional_rate", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolNetQty, Name: "cumulative_volume_delta", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolNetNotional, Name: "cumulative_notional_delta", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: temporal.SymbolObservedSec, Name: "cvd_epoch_from", Unit: data.UnitSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: preBaselineSlot(prefixNetFrac), Name: "signed_net_fraction_baseline", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: prefixed(prefixNetFrac, "z/residual"), Name: "signed_net_fraction_divergence", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: prefixed(prefixNetFrac, "z/value"), Name: "signed_net_fraction_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolGrossBase, Name: "gross_notional_rate_baseline", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolGrossRatio, Name: "gross_notional_rate_ratio", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: prefixed(prefixGrossRate, "z/residual"), Name: "gross_notional_rate_divergence", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: prefixed(prefixGrossRate, "z/value"), Name: "gross_notional_rate_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolNetVelocity, Name: "net_notional_rate_velocity", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolGrossVelocity, Name: "gross_notional_rate_velocity", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolMidpointFrom, Name: "response_midpoint:from", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolMidpoint, Name: "response_midpoint:at", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolMidpointLog, Name: "midpoint_log_return", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolMidpointRate, Name: "midpoint_return_rate", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolFlowAligned, Name: "flow_aligned_midpoint_return", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolResponsePerNet, Name: "midpoint_response_per_net_notional", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: preBaselineSlot(prefixMidRate), Name: "midpoint_return_rate_baseline", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: prefixed(prefixMidRate, "z/residual"), Name: "midpoint_return_rate_divergence", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: prefixed(prefixMidRate, "z/value"), Name: "midpoint_return_rate_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		),
	}
}

/*
Step receives one trade, loads its facts and the contemporaneous quote, runs
the Number pipeline, and projects exactly one Measurement. Validation happens
inside the pipeline; any invalid input surfaces as a pipeline failure carried
on the Measurement's Err field rather than a Go error return.

The shared book supplies the response-price quote when one exists. A missing
book or an empty/crossed touch is an undefined quote, not an error: the flow
accounting still stands and the response-price metrics are simply absent.
*/
func (trade *Trade) Step(observation kraken.TradeData) *data.Measurement[float64] {
	input := nmtypes.Frame{}
	input.Put(symbolPrice, observation.Price.Float64())
	input.Put(symbolQty, observation.Qty)
	input.Put(symbolSign, signForSide(observation.Side))
	input.Put(nmtypes.EventTimeSec, float64(observation.Timestamp.Unix()))
	input.Put(nmtypes.EventTimeNsec, float64(observation.Timestamp.Nanosecond()))

	trade.loadQuote(observation.Symbol, &input)

	output := trade.number.Step(observation.Symbol, input)

	from := observation.Timestamp

	if seconds, found := output.Get(temporal.SymbolObservedSec); found {
		if nanoseconds, foundNanoseconds := output.Get(temporal.SymbolObservedNsec); foundNanoseconds {
			from = time.Unix(int64(seconds), int64(nanoseconds))
		}
	}

	return trade.projector.Project(observation.Symbol, "cvd", observation.Timestamp, from, output)
}

/*
loadQuote loads the latest valid quote midpoint from the shared book when one
is available and seeds the epoch midpoint on the first valid quote.
*/
func (trade *Trade) loadQuote(symbol string, input *nmtypes.Frame) {
	if trade == nil || trade.workspace == nil {
		input.Put(symbolHasQuote, 0)
		return
	}

	var hasQuote bool
	var bidPrice, askPrice float64

	inspectBook := func(resident *book.Book) {
		if resident == nil {
			return
		}
		bestBid := resident.BestBid()
		bestAsk := resident.BestAsk()

		if bestBid != nil && bestAsk != nil && bestBid.Price != nil && bestAsk.Price != nil {
			bidPrice = bestBid.Price.Float64()
			askPrice = bestAsk.Price.Float64()

			if bidPrice > 0 && askPrice > bidPrice {
				hasQuote = true
			}
		}
	}

	if sharedBook, found := trade.workspace.Shared("book", symbol); found && sharedBook != nil {
		if currentBook, ok := sharedBook.(*book.Book); ok && currentBook != nil {
			inspectBook(currentBook)
		}
	} else if shared, found := trade.workspace.Shared("api", ""); found && shared != nil {
		if api, ok := shared.(*websocket.API); ok && api != nil {
			api.Book(symbol, inspectBook)
		}
	}

	if !hasQuote {
		input.Put(symbolHasQuote, 0)
		return
	}

	input.Put(symbolHasQuote, 1)
	input.Put(symbolBidPrice, bidPrice)
	input.Put(symbolAskPrice, askPrice)

	committed, committedFound := trade.number.Project(symbol)

	if !committedFound || !committed.Has(symbolBidFrom) {
		input.Put(symbolBidFrom, bidPrice)
		input.Put(symbolAskFrom, askPrice)
	}
}

func (trade *Trade) Close() error { return nil }

/*
signForSide encodes one trade's aggressor side into a signed marker: buys are
positive, every other side is negative.
*/
func signForSide(side string) float64 {
	if side == "buy" {
		return 1
	}

	return -1
}
