package liquidity

import (
	"fmt"

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
Input/output slot symbols for the touch metric pipeline.
*/
var (
	symbolBidPrice         = nmtypes.MustIntern("liquidity/bid_price")
	symbolAskPrice         = nmtypes.MustIntern("liquidity/ask_price")
	symbolBidQty           = nmtypes.MustIntern("liquidity/bid_qty")
	symbolAskQty           = nmtypes.MustIntern("liquidity/ask_qty")
	symbolMidpoint         = nmtypes.MustIntern("liquidity/midpoint")
	symbolSpread           = nmtypes.MustIntern("liquidity/spread")
	symbolRelativeSpread   = nmtypes.MustIntern("liquidity/relative_spread")
	symbolBidNotional      = nmtypes.MustIntern("liquidity/touch_notional_bid")
	symbolAskNotional      = nmtypes.MustIntern("liquidity/touch_notional_ask")
	symbolTwoSidedNotional = nmtypes.MustIntern("liquidity/two_sided_notional")
	symbolSumNotional      = nmtypes.MustIntern("liquidity/sum_notional")
	symbolDiffNotional     = nmtypes.MustIntern("liquidity/diff_notional")
	symbolImbalance        = nmtypes.MustIntern("liquidity/touch_notional_imbalance")

	// Estimator chain slot tables (namespaced, one per measured quantity).
	depthBidSlots = newEstimatorSlots("touch_bid")
	depthAskSlots = newEstimatorSlots("touch_ask")
	spreadSlots   = newEstimatorSlots("relative_spread")

	// Log-space intermediates and estimator outputs.
	symbolLogDepthBid           = nmtypes.MustIntern("liquidity/log_depth_bid")
	symbolLogDepthAsk           = nmtypes.MustIntern("liquidity/log_depth_ask")
	symbolLogRelativeSpread     = nmtypes.MustIntern("liquidity/log_relative_spread")
	symbolDepthBidBaseline      = nmtypes.MustIntern("liquidity/touch_notional_baseline_bid")
	symbolDepthBidRatio         = nmtypes.MustIntern("liquidity/depth_ratio_bid")
	symbolDepthBidDivergenceVel = nmtypes.MustIntern("liquidity/divergence_velocity_bid")
	symbolDepthAskBaseline      = nmtypes.MustIntern("liquidity/touch_notional_baseline_ask")
	symbolDepthAskRatio         = nmtypes.MustIntern("liquidity/depth_ratio_ask")
	symbolDepthAskDivergenceVel = nmtypes.MustIntern("liquidity/divergence_velocity_ask")
	symbolRelativeSpreadBase    = nmtypes.MustIntern("liquidity/relative_spread_baseline")
	symbolSpreadRatio           = nmtypes.MustIntern("liquidity/spread_ratio")
	symbolSpreadDivergenceVel   = nmtypes.MustIntern("liquidity/spread_divergence_velocity")

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
logEstimator routes one strictly positive quantity through log space into its
namespaced series and runs the causal ZScore → Baseline chain, then projects the
baseline back into original units and forms the current-to-baseline ratio.

The liquidity depth and spread families are positive and multiplicative by
construction (signal/liquidity/README.md §6/§7), so historical comparison runs
in log space: the baseline is the causal pre-observation mean, the residual is
the log divergence d = log(value) - log(baseline), the dispersion is the
event-time decayed residual RMS (the noise scale), and the z-score is the
residual standardized by that dispersion.
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
event-clock first difference per second. The first observation seeds the
differencer and produces no rate; the gate keeps the rate absent rather than
dividing by zero.
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
Ticker is the touch-snapshot market entity. It owns exactly a Number pipeline
and a projector, both declared in its constructor, plus Step and Close.
*/
type Ticker struct {
	number    *nomagique.Number[string]
	projector *data.Projector
}

/*
NewTicker constructs the Ticker entity: one Number pipeline for the touch
metric computation and one projector that names the output slots.
*/
func NewTicker() *Ticker {
	return &Ticker{
		number: nomagique.NewNumber[string](nmtypes.Pipe(
			// 0 < bid < ask: a crossed, missing, or non-positive book is rejected here.
			logic.PositiveOrder(symbolBidPrice, symbolAskPrice),
			// Bid notional: Db = bidPrice * bidQty
			nmtypes.Wire(
				calculus.Product,
				nmtypes.In(symbolBidPrice, calculus.PortA),
				nmtypes.In(symbolBidQty, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolBidNotional),
			),
			// Ask notional: Da = askPrice * askQty
			nmtypes.Wire(
				calculus.Product,
				nmtypes.In(symbolAskPrice, calculus.PortA),
				nmtypes.In(symbolAskQty, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolAskNotional),
			),
			// Midpoint: (bid+ask)/2
			nmtypes.Wire(
				calculus.Average,
				nmtypes.In(symbolBidPrice, calculus.PortA),
				nmtypes.In(symbolAskPrice, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolMidpoint),
			),
			// Spread: ask-bid
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(symbolAskPrice, calculus.PortA),
				nmtypes.In(symbolBidPrice, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolSpread),
			),
			// Relative spread: spread / midpoint
			nmtypes.Wire(
				calculus.Quotient,
				nmtypes.In(symbolSpread, calculus.PortA),
				nmtypes.In(symbolMidpoint, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolRelativeSpread),
			),
			// Two-sided notional: min(Db, Da)
			nmtypes.Wire(
				calculus.Minimum,
				nmtypes.In(symbolBidNotional, calculus.PortA),
				nmtypes.In(symbolAskNotional, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolTwoSidedNotional),
			),
			// Sum notional: Db + Da
			nmtypes.Wire(
				calculus.Sum,
				nmtypes.In(symbolBidNotional, calculus.PortA),
				nmtypes.In(symbolAskNotional, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolSumNotional),
			),
			// Difference notional: Db - Da
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(symbolBidNotional, calculus.PortA),
				nmtypes.In(symbolAskNotional, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolDiffNotional),
			),
			// Imbalance: (Db - Da) / (Db + Da)
			nmtypes.Wire(
				calculus.Quotient,
				nmtypes.In(symbolDiffNotional, calculus.PortA),
				nmtypes.In(symbolSumNotional, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolImbalance),
			),

			// Bid depth historical family (log space) and its divergence
			// velocity, gated on the causal baseline.
			logEstimator(depthBidSlots, symbolBidNotional, symbolLogDepthBid, symbolDepthBidBaseline, symbolDepthBidRatio),
			logic.If(
				readyPredicate(depthBidSlots.ready),
				velocityChain("touch_bid_divergence_velocity", depthBidSlots.residual, symbolDepthBidDivergenceVel),
				nil,
			),

			// Ask depth historical family (log space) and its divergence
			// velocity, gated on the causal baseline.
			logEstimator(depthAskSlots, symbolAskNotional, symbolLogDepthAsk, symbolDepthAskBaseline, symbolDepthAskRatio),
			logic.If(
				readyPredicate(depthAskSlots.ready),
				velocityChain("touch_ask_divergence_velocity", depthAskSlots.residual, symbolDepthAskDivergenceVel),
				nil,
			),

			// Relative spread historical family (log space) and its divergence
			// velocity, gated on the causal baseline.
			logEstimator(spreadSlots, symbolRelativeSpread, symbolLogRelativeSpread, symbolRelativeSpreadBase, symbolSpreadRatio),
			logic.If(
				readyPredicate(spreadSlots.ready),
				velocityChain("spread_divergence_velocity", spreadSlots.residual, symbolSpreadDivergenceVel),
				nil,
			),

			// Estimator quality facts for data.Measurement.Finalize, gated on
			// the bid-depth z-score readiness: surface the estimator support as
			// the sample count (so Finalize derives Maturity = 1 - 1/support),
			// and the divergence / noise variance from the causal estimator.
			nmtypes.Wire(
				nmtypes.Identity,
				nmtypes.In(depthBidSlots.series.CountSymbol, nmtypes.SampleCount),
				nmtypes.Out(nmtypes.SampleCount, nmtypes.SampleCount),
			),
			logic.If(
				readyPredicate(depthBidSlots.ready),
				nmtypes.Pipe(
					nmtypes.Wire(
						nmtypes.Identity,
						nmtypes.In(depthBidSlots.residual, symbolDivergence),
						nmtypes.Out(symbolDivergence, symbolDivergence),
					),
					nmtypes.Wire(
						calculus.Product,
						nmtypes.In(depthBidSlots.dispersion, calculus.PortA),
						nmtypes.In(depthBidSlots.dispersion, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolNoiseVariance),
					),
				),
				nil,
			),
		)),
		projector: data.NewProjector(
			data.Binding{From: symbolBidPrice, Name: "best_bid_price", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskPrice, Name: "best_ask_price", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBidQty, Name: "touch_quantity:bid", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskQty, Name: "touch_quantity:ask", Unit: data.UnitCount, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBidNotional, Name: "touch_notional:bid", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskNotional, Name: "touch_notional:ask", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolMidpoint, Name: "midpoint", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolSpread, Name: "spread", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolRelativeSpread, Name: "relative_spread", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolTwoSidedNotional, Name: "two_sided_touch_notional", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolImbalance, Name: "touch_notional_imbalance", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},

			// Bid depth historical family.
			data.Binding{From: symbolDepthBidBaseline, Name: "touch_notional_baseline:bid", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolDepthBidRatio, Name: "depth_ratio:bid", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: depthBidSlots.residual, Name: "depth_divergence:bid", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: depthBidSlots.dispersion, Name: "depth_noise_scale:bid", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: depthBidSlots.zscore, Name: "depth_zscore:bid", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolDepthBidDivergenceVel, Name: "divergence_velocity:bid", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},

			// Ask depth historical family.
			data.Binding{From: symbolDepthAskBaseline, Name: "touch_notional_baseline:ask", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolDepthAskRatio, Name: "depth_ratio:ask", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: depthAskSlots.residual, Name: "depth_divergence:ask", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: depthAskSlots.dispersion, Name: "depth_noise_scale:ask", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: depthAskSlots.zscore, Name: "depth_zscore:ask", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolDepthAskDivergenceVel, Name: "divergence_velocity:ask", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},

			// Spread historical family.
			data.Binding{From: symbolRelativeSpreadBase, Name: "relative_spread_baseline", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolSpreadRatio, Name: "spread_ratio", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: spreadSlots.residual, Name: "spread_divergence", Unit: data.UnitNat, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: spreadSlots.zscore, Name: "spread_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolSpreadDivergenceVel, Name: "spread_divergence_velocity", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
		),
	}
}

/*
Step receives one market data point, loads the touch facts, runs the Number
pipeline, and projects exactly one Measurement. Validation happens inside the
pipeline; any invalid input surfaces as a pipeline failure carried on the
Measurement's own Err field rather than a Go error return.
*/
func (ticker *Ticker) Step(trade kraken.TickerData) *data.Measurement[float64] {
	if trade.Bid == nil || trade.Ask == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf("liquidity: ticker requires bid and ask")}
	}

	input := nmtypes.Frame{}
	input.Put(symbolBidPrice, trade.Bid.Float64())
	input.Put(symbolAskPrice, trade.Ask.Float64())
	input.Put(symbolBidQty, trade.BidQty)
	input.Put(symbolAskQty, trade.AskQty)
	input.Put(nmtypes.EventTimeSec, float64(trade.Timestamp.Unix()))
	input.Put(nmtypes.EventTimeNsec, float64(trade.Timestamp.Nanosecond()))

	return ticker.projector.Project(
		trade.Symbol,
		"liquidity",
		trade.Timestamp,
		trade.Timestamp,
		ticker.number.Step(trade.Symbol, input),
	)
}

func (ticker *Ticker) Close() error { return nil }
