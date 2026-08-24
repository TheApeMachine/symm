package exhaust

import (
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
Input/output slot symbols for the support-state metric pipeline. The "previous"
slots carry the prior observation's facts so the current and prior touch
notionals can be compared inside one frame.
*/
var (
	symbolBidPrice        = nmtypes.MustIntern("exhaust/bid_price")
	symbolAskPrice        = nmtypes.MustIntern("exhaust/ask_price")
	symbolBidQty          = nmtypes.MustIntern("exhaust/bid_qty")
	symbolAskQty          = nmtypes.MustIntern("exhaust/ask_qty")
	symbolPrevBid         = nmtypes.MustIntern("exhaust/previous_bid_price")
	symbolPrevAsk         = nmtypes.MustIntern("exhaust/previous_ask_price")
	symbolPrevBidQty      = nmtypes.MustIntern("exhaust/previous_bid_qty")
	symbolPrevAskQty      = nmtypes.MustIntern("exhaust/previous_ask_qty")
	symbolDepthBid        = nmtypes.MustIntern("exhaust/depth_notional_bid")
	symbolDepthAsk        = nmtypes.MustIntern("exhaust/depth_notional_ask")
	symbolDepthTotal      = nmtypes.MustIntern("exhaust/depth_notional")
	symbolDepthDiff       = nmtypes.MustIntern("exhaust/depth_diff")
	symbolMidpoint        = nmtypes.MustIntern("exhaust/midpoint")
	symbolSpread          = nmtypes.MustIntern("exhaust/spread")
	symbolRelSpread       = nmtypes.MustIntern("exhaust/relative_spread")
	symbolImbalance       = nmtypes.MustIntern("exhaust/book_imbalance")
	symbolPrevDepthBid    = nmtypes.MustIntern("exhaust/prev_depth_notional_bid")
	symbolPrevDepthAsk    = nmtypes.MustIntern("exhaust/prev_depth_notional_ask")
	symbolPrevDepthTot    = nmtypes.MustIntern("exhaust/prev_depth_notional")
	symbolPrevDepthDiff   = nmtypes.MustIntern("exhaust/prev_depth_diff")
	symbolPrevImbalance   = nmtypes.MustIntern("exhaust/previous_book_imbalance")
	symbolImbalanceChange = nmtypes.MustIntern("exhaust/book_imbalance_change")
	symbolPrevMidpoint    = nmtypes.MustIntern("exhaust/previous_midpoint")
	symbolMidpointReturn  = nmtypes.MustIntern("exhaust/midpoint_log_return")

	// Estimator chain slot tables (namespaced, one per measured quantity).
	bookImbalanceSlots = newEstimatorSlots("book_imbalance")
	depthBidSlots      = newEstimatorSlots("depth_bid")
	depthAskSlots      = newEstimatorSlots("depth_ask")
	totalDepthSlots    = newEstimatorSlots("total_depth")
	spreadSlots        = newEstimatorSlots("spread")

	// Log-space intermediates and estimator outputs.
	symbolLogDepthBid           = nmtypes.MustIntern("exhaust/log_depth_bid")
	symbolLogDepthAsk           = nmtypes.MustIntern("exhaust/log_depth_ask")
	symbolLogTotalDepth         = nmtypes.MustIntern("exhaust/log_total_depth")
	symbolLogSpread             = nmtypes.MustIntern("exhaust/log_relative_spread")
	symbolDepthBidBaseline      = nmtypes.MustIntern("exhaust/depth_baseline_bid")
	symbolDepthBidRatio         = nmtypes.MustIntern("exhaust/depth_ratio_bid")
	symbolDepthBidDivergenceVel = nmtypes.MustIntern("exhaust/depth_divergence_velocity_bid")
	symbolDepthAskBaseline      = nmtypes.MustIntern("exhaust/depth_baseline_ask")
	symbolDepthAskRatio         = nmtypes.MustIntern("exhaust/depth_ratio_ask")
	symbolDepthAskDivergenceVel = nmtypes.MustIntern("exhaust/depth_divergence_velocity_ask")
	symbolTotalDepthBaseline    = nmtypes.MustIntern("exhaust/total_depth_baseline")
	symbolTotalDepthRatio       = nmtypes.MustIntern("exhaust/total_depth_ratio")
	symbolSpreadBaseline        = nmtypes.MustIntern("exhaust/relative_spread_baseline")
	symbolSpreadRatio           = nmtypes.MustIntern("exhaust/spread_ratio")
	symbolSpreadDivergenceVel   = nmtypes.MustIntern("exhaust/spread_divergence_velocity")
	symbolBookImbalanceVelocity = nmtypes.MustIntern("exhaust/book_imbalance_velocity")

	symbolDivergence    = nmtypes.MustIntern("divergence")
	symbolNoiseVariance = nmtypes.MustIntern("noise_variance")
)

/*
estimatorSlots resolves the namespaced slot table one estimator chain writes.
Every estimator (baseline, z-score, velocity) is keyed by the same prefix, so
several independent chains can share one frame without slot collisions.
*/
type estimatorSlots struct {
	prefix     string
	series     temporal.Series
	baseline   nmtypes.Symbol
	residual   nmtypes.Symbol
	dispersion nmtypes.Symbol
	zscore     nmtypes.Symbol
	ready      nmtypes.Symbol
}

func newEstimatorSlots(prefix string) estimatorSlots {
	return estimatorSlots{
		prefix:     prefix,
		series:     temporal.NewSeries(prefix),
		baseline:   nmtypes.MustIntern(temporal.JoinPrefix(prefix, "baseline/value")),
		residual:   nmtypes.MustIntern(temporal.JoinPrefix(prefix, "z/residual")),
		dispersion: nmtypes.MustIntern(temporal.JoinPrefix(prefix, "z/dispersion")),
		zscore:     nmtypes.MustIntern(temporal.JoinPrefix(prefix, "z/value")),
		ready:      nmtypes.MustIntern(temporal.JoinPrefix(prefix, "z/ready")),
	}
}

/*
readyPredicate turns one estimator's captured z-score readiness into a logic
condition so downstream stages run only once a causal baseline exists.
*/
func readyPredicate(ready nmtypes.Symbol) nmtypes.Primitive {
	return nmtypes.Wire(
		nmtypes.Identity,
		nmtypes.In(ready, logic.SymbolCondition),
		nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
	)
}

/*
additiveEstimator routes one bounded additive quantity into its namespaced
series and runs the causal ZScore → Baseline chain.
*/
func additiveEstimator(slots estimatorSlots, source nmtypes.Symbol) nmtypes.Primitive {
	return nmtypes.Pipe(
		nmtypes.Wire(
			nmtypes.Identity,
			nmtypes.In(source, slots.series.ValueSymbol),
			nmtypes.Out(slots.series.ValueSymbol, slots.series.ValueSymbol),
		),
		nmtypes.Wire(
			nmtypes.Identity,
			nmtypes.In(nmtypes.EventTimeSec, slots.series.SecSymbol),
			nmtypes.Out(slots.series.SecSymbol, slots.series.SecSymbol),
		),
		nmtypes.Wire(
			nmtypes.Identity,
			nmtypes.In(nmtypes.EventTimeNsec, slots.series.NsecSymbol),
			nmtypes.Out(slots.series.NsecSymbol, slots.series.NsecSymbol),
		),
		statistic.ZScore(slots.prefix),
		nmtypes.Wire(
			nmtypes.Identity,
			nmtypes.In(slots.series.ReadySymbol, slots.ready),
			nmtypes.Out(slots.ready, slots.ready),
		),
		nmtypes.Configure(
			statistic.Baseline(slots.prefix),
			slots.series.SpanSymbol,
			temporal.Window(slots.prefix),
		),
	)
}

/*
logEstimator routes one strictly positive quantity through log space into its
namespaced series and runs the causal ZScore → Baseline chain, then projects the
baseline back into original units and forms the current-to-baseline ratio.
*/
func logEstimator(
	slots estimatorSlots,
	source nmtypes.Symbol,
	logSource nmtypes.Symbol,
	expBaseline nmtypes.Symbol,
	ratio nmtypes.Symbol,
) nmtypes.Primitive {
	return nmtypes.Pipe(
		nmtypes.Wire(
			calculus.Log,
			nmtypes.In(source, calculus.PortX),
			nmtypes.Out(calculus.PortResult, logSource),
		),
		nmtypes.Wire(
			nmtypes.Identity,
			nmtypes.In(logSource, slots.series.ValueSymbol),
			nmtypes.Out(slots.series.ValueSymbol, slots.series.ValueSymbol),
		),
		nmtypes.Wire(
			nmtypes.Identity,
			nmtypes.In(nmtypes.EventTimeSec, slots.series.SecSymbol),
			nmtypes.Out(slots.series.SecSymbol, slots.series.SecSymbol),
		),
		nmtypes.Wire(
			nmtypes.Identity,
			nmtypes.In(nmtypes.EventTimeNsec, slots.series.NsecSymbol),
			nmtypes.Out(slots.series.NsecSymbol, slots.series.NsecSymbol),
		),
		statistic.ZScore(slots.prefix),
		nmtypes.Wire(
			nmtypes.Identity,
			nmtypes.In(slots.series.ReadySymbol, slots.ready),
			nmtypes.Out(slots.ready, slots.ready),
		),
		nmtypes.Configure(
			statistic.Baseline(slots.prefix),
			slots.series.SpanSymbol,
			temporal.Window(slots.prefix),
		),
		nmtypes.Wire(
			calculus.Exp,
			nmtypes.In(slots.baseline, calculus.PortX),
			nmtypes.Out(calculus.PortResult, expBaseline),
		),
		nmtypes.Wire(
			calculus.Quotient,
			nmtypes.In(source, calculus.PortA),
			nmtypes.In(expBaseline, calculus.PortB),
			nmtypes.Out(calculus.PortResult, ratio),
		),
	)
}

/*
velocityChain routes one quantity into its own namespaced series and emits its
event-clock first difference per second.
*/
func velocityChain(prefix string, source nmtypes.Symbol, rate nmtypes.Symbol) nmtypes.Primitive {
	series := temporal.NewSeries(prefix)
	delta := nmtypes.MustIntern(temporal.JoinPrefix(prefix, "velocity/delta"))
	elapsed := nmtypes.MustIntern(temporal.JoinPrefix(prefix, "velocity/elapsed_sec"))

	return nmtypes.Pipe(
		nmtypes.Wire(
			nmtypes.Identity,
			nmtypes.In(source, series.ValueSymbol),
			nmtypes.Out(series.ValueSymbol, series.ValueSymbol),
		),
		nmtypes.Wire(
			nmtypes.Identity,
			nmtypes.In(nmtypes.EventTimeSec, series.SecSymbol),
			nmtypes.Out(series.SecSymbol, series.SecSymbol),
		),
		nmtypes.Wire(
			nmtypes.Identity,
			nmtypes.In(nmtypes.EventTimeNsec, series.NsecSymbol),
			nmtypes.Out(series.NsecSymbol, series.NsecSymbol),
		),
		statistic.Velocity(prefix),
		logic.If(
			nmtypes.Wire(
				nmtypes.Identity,
				nmtypes.In(series.ReadySymbol, logic.SymbolCondition),
				nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
			),
			nmtypes.Wire(
				calculus.Quotient,
				nmtypes.In(delta, calculus.PortA),
				nmtypes.In(elapsed, calculus.PortB),
				nmtypes.Out(calculus.PortResult, rate),
			),
			nil,
		),
	)
}

/*
Ticker is the touch-snapshot market entity for the support-state compatibility
signal. It owns exactly a Number pipeline and a projector, both declared in its
constructor, plus Step and Close.
*/
type Ticker struct {
	number    *nomagique.Number[string]
	projector *data.Projector
}

/*
NewTicker constructs the Ticker entity: one Number pipeline for the depth,
spread, and imbalance metric computation and one projector that names the
output slots.
*/
func NewTicker() *Ticker {
	return &Ticker{
		number: nomagique.NewNumber[string](nmtypes.Pipe(
			// 0 < bid < ask: a crossed, missing, or non-positive book is rejected here.
			logic.PositiveOrder(symbolBidPrice, symbolAskPrice),

			// Current touch depth notionals and their asymmetry.
			nmtypes.Wire(
				calculus.Product,
				nmtypes.In(symbolBidPrice, calculus.PortA),
				nmtypes.In(symbolBidQty, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolDepthBid),
			),
			nmtypes.Wire(
				calculus.Product,
				nmtypes.In(symbolAskPrice, calculus.PortA),
				nmtypes.In(symbolAskQty, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolDepthAsk),
			),
			nmtypes.Wire(
				calculus.Sum,
				nmtypes.In(symbolDepthBid, calculus.PortA),
				nmtypes.In(symbolDepthAsk, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolDepthTotal),
			),
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(symbolDepthBid, calculus.PortA),
				nmtypes.In(symbolDepthAsk, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolDepthDiff),
			),
			nmtypes.Wire(
				calculus.Quotient,
				nmtypes.In(symbolDepthDiff, calculus.PortA),
				nmtypes.In(symbolDepthTotal, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolImbalance),
			),

			// Spread and relative spread.
			nmtypes.Wire(
				calculus.Average,
				nmtypes.In(symbolBidPrice, calculus.PortA),
				nmtypes.In(symbolAskPrice, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolMidpoint),
			),
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(symbolAskPrice, calculus.PortA),
				nmtypes.In(symbolBidPrice, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolSpread),
			),
			nmtypes.Wire(
				calculus.Quotient,
				nmtypes.In(symbolSpread, calculus.PortA),
				nmtypes.In(symbolMidpoint, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolRelSpread),
			),

			// Previous touch depth notionals and prior imbalance.
			nmtypes.Wire(
				calculus.Product,
				nmtypes.In(symbolPrevBid, calculus.PortA),
				nmtypes.In(symbolPrevBidQty, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolPrevDepthBid),
			),
			nmtypes.Wire(
				calculus.Product,
				nmtypes.In(symbolPrevAsk, calculus.PortA),
				nmtypes.In(symbolPrevAskQty, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolPrevDepthAsk),
			),
			nmtypes.Wire(
				calculus.Sum,
				nmtypes.In(symbolPrevDepthBid, calculus.PortA),
				nmtypes.In(symbolPrevDepthAsk, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolPrevDepthTot),
			),
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(symbolPrevDepthBid, calculus.PortA),
				nmtypes.In(symbolPrevDepthAsk, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolPrevDepthDiff),
			),
			nmtypes.Wire(
				calculus.Quotient,
				nmtypes.In(symbolPrevDepthDiff, calculus.PortA),
				nmtypes.In(symbolPrevDepthTot, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolPrevImbalance),
			),
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(symbolImbalance, calculus.PortA),
				nmtypes.In(symbolPrevImbalance, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolImbalanceChange),
			),

			// Midpoint log return against the prior midpoint.
			nmtypes.Wire(
				calculus.Average,
				nmtypes.In(symbolPrevBid, calculus.PortA),
				nmtypes.In(symbolPrevAsk, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolPrevMidpoint),
			),
			nmtypes.Wire(
				calculus.LogRatio,
				nmtypes.In(symbolMidpoint, calculus.SymbolCurrent),
				nmtypes.In(symbolPrevMidpoint, calculus.SymbolPrevious),
				nmtypes.Out(calculus.PortResult, symbolMidpointReturn),
			),

			// Book imbalance estimator chain and velocity.
			additiveEstimator(bookImbalanceSlots, symbolImbalance),
			// Surface the book-imbalance estimator support as the measurement's
			// sample count so Finalize derives Maturity (1 - 1/support).
			nmtypes.Wire(
				nmtypes.Identity,
				nmtypes.In(bookImbalanceSlots.series.CountSymbol, nmtypes.SampleCount),
				nmtypes.Out(nmtypes.SampleCount, nmtypes.SampleCount),
			),
			velocityChain("book_imbalance_velocity", symbolImbalance, symbolBookImbalanceVelocity),

			// Side-specific depth estimator chains (log-space), with their
			// divergence velocities gated on the causal baseline.
			logEstimator(depthBidSlots, symbolDepthBid, symbolLogDepthBid, symbolDepthBidBaseline, symbolDepthBidRatio),
			logic.If(
				readyPredicate(depthBidSlots.ready),
				velocityChain("depth_bid_divergence_velocity", depthBidSlots.residual, symbolDepthBidDivergenceVel),
				nil,
			),
			logEstimator(depthAskSlots, symbolDepthAsk, symbolLogDepthAsk, symbolDepthAskBaseline, symbolDepthAskRatio),
			logic.If(
				readyPredicate(depthAskSlots.ready),
				velocityChain("depth_ask_divergence_velocity", depthAskSlots.residual, symbolDepthAskDivergenceVel),
				nil,
			),

			// Total displayed depth estimator chain (log-space).
			logEstimator(totalDepthSlots, symbolDepthTotal, symbolLogTotalDepth, symbolTotalDepthBaseline, symbolTotalDepthRatio),

			// Relative spread estimator chain (log-space) and its divergence
			// velocity.
			logEstimator(spreadSlots, symbolRelSpread, symbolLogSpread, symbolSpreadBaseline, symbolSpreadRatio),
			logic.If(
				readyPredicate(spreadSlots.ready),
				velocityChain("spread_divergence_velocity", spreadSlots.residual, symbolSpreadDivergenceVel),
				nil,
			),

			// Estimator quality facts for data.Measurement.Finalize, gated on
			// the book-imbalance z-score readiness.
			logic.If(
				readyPredicate(bookImbalanceSlots.ready),
				nmtypes.Pipe(
					nmtypes.Wire(
						nmtypes.Identity,
						nmtypes.In(bookImbalanceSlots.residual, symbolDivergence),
						nmtypes.Out(symbolDivergence, symbolDivergence),
					),
					nmtypes.Wire(
						calculus.Product,
						nmtypes.In(bookImbalanceSlots.dispersion, calculus.PortA),
						nmtypes.In(bookImbalanceSlots.dispersion, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolNoiseVariance),
					),
				),
				nil,
			),
		)),
		projector: data.NewProjector(
			data.Binding{From: symbolDepthBid, Name: "displayed_depth_notional:bid", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolDepthAsk, Name: "displayed_depth_notional:ask", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolDepthTotal, Name: "displayed_depth_notional", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolSpread, Name: "spread", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolRelSpread, Name: "relative_spread", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolImbalance, Name: "book_imbalance", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolPrevImbalance, Name: "previous_book_imbalance", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolImbalanceChange, Name: "book_imbalance_change", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolMidpointReturn, Name: "midpoint_log_return", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: bookImbalanceSlots.baseline, Name: "book_imbalance_baseline", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: bookImbalanceSlots.zscore, Name: "book_imbalance_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBookImbalanceVelocity, Name: "book_imbalance_velocity", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolDepthBidBaseline, Name: "depth_baseline:bid", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: depthBidSlots.residual, Name: "depth_divergence:bid", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolDepthBidDivergenceVel, Name: "depth_divergence_velocity:bid", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolDepthBidRatio, Name: "depth_ratio:bid", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: depthBidSlots.zscore, Name: "depth_zscore:bid", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolDepthAskBaseline, Name: "depth_baseline:ask", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: depthAskSlots.residual, Name: "depth_divergence:ask", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolDepthAskDivergenceVel, Name: "depth_divergence_velocity:ask", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolDepthAskRatio, Name: "depth_ratio:ask", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: depthAskSlots.zscore, Name: "depth_zscore:ask", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolTotalDepthBaseline, Name: "total_depth_baseline", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolTotalDepthRatio, Name: "total_depth_ratio", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: totalDepthSlots.zscore, Name: "total_depth_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolSpreadBaseline, Name: "relative_spread_baseline", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: spreadSlots.residual, Name: "spread_divergence", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolSpreadDivergenceVel, Name: "spread_divergence_velocity", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolSpreadRatio, Name: "spread_ratio", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: spreadSlots.zscore, Name: "spread_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		),
	}
}

/*
Step receives one ticker data point, loads the touch facts, runs the Number
pipeline, and projects exactly one Measurement. Validation happens inside the
pipeline; any invalid input surfaces as a pipeline failure carried on the
Measurement's own Err field rather than a Go error return.
*/
func (ticker *Ticker) Step(trade kraken.TickerData) *data.Measurement[float64] {
	input := nmtypes.Frame{}
	input.Put(symbolBidPrice, trade.Bid.Float64())
	input.Put(symbolAskPrice, trade.Ask.Float64())
	input.Put(symbolBidQty, trade.BidQty)
	input.Put(symbolAskQty, trade.AskQty)
	input.Put(nmtypes.EventTimeSec, float64(trade.Timestamp.Unix()))
	input.Put(nmtypes.EventTimeNsec, float64(trade.Timestamp.Nanosecond()))

	previousBid := trade.Bid.Float64()
	previousAsk := trade.Ask.Float64()
	previousBidQty := trade.BidQty
	previousAskQty := trade.AskQty

	if committed, found := ticker.number.Project(trade.Symbol); found {
		previousBid, _ = committed.Get(symbolBidPrice)
		previousAsk, _ = committed.Get(symbolAskPrice)
		previousBidQty, _ = committed.Get(symbolBidQty)
		previousAskQty, _ = committed.Get(symbolAskQty)
	}

	input.Put(symbolPrevBid, previousBid)
	input.Put(symbolPrevAsk, previousAsk)
	input.Put(symbolPrevBidQty, previousBidQty)
	input.Put(symbolPrevAskQty, previousAskQty)

	return ticker.projector.Project(
		trade.Symbol,
		"exhaust",
		trade.Timestamp,
		trade.Timestamp,
		ticker.number.Step(trade.Symbol, input),
	)
}

func (ticker *Ticker) Close() error { return nil }
