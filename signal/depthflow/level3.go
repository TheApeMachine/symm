package depthflow

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
Current and previous observation slots for the two notional streams the flow
metrics compare inside one frame. The "previous" and "current" prefixes follow
the temporal series convention.
*/
var (
	currentBidSeries  = temporal.NewSeries("current/bid_notional")
	currentAskSeries  = temporal.NewSeries("current/ask_notional")
	previousBidSeries = temporal.NewSeries("previous/bid_notional")
	previousAskSeries = temporal.NewSeries("previous/ask_notional")

	symbolBookNotionalBid  = currentBidSeries.ValueSymbol
	symbolBookNotionalAsk  = currentAskSeries.ValueSymbol
	symbolPrevBidNotional  = previousBidSeries.ValueSymbol
	symbolPrevAskNotional  = previousAskSeries.ValueSymbol
	symbolTouchBidPrice    = nmtypes.MustIntern("depthflow/touch_bid_price")
	symbolTouchAskPrice    = nmtypes.MustIntern("depthflow/touch_ask_price")
	symbolTouchNotionalBid = nmtypes.MustIntern("depthflow/touch_notional_bid")
	symbolTouchNotionalAsk = nmtypes.MustIntern("depthflow/touch_notional_ask")

	symbolBookNotional      = nmtypes.MustIntern("depthflow/book_notional")
	symbolBookDiff          = nmtypes.MustIntern("depthflow/book_diff")
	symbolBookImbalance     = nmtypes.MustIntern("depthflow/book_imbalance")
	symbolTouchDiff         = nmtypes.MustIntern("depthflow/touch_diff")
	symbolTouchNotional     = nmtypes.MustIntern("depthflow/touch_notional")
	symbolTouchImbalance    = nmtypes.MustIntern("depthflow/touch_imbalance")
	symbolResolutionGap     = nmtypes.MustIntern("depthflow/imbalance_resolution_gap")
	symbolResolutionDist    = nmtypes.MustIntern("depthflow/imbalance_resolution_distance")
	symbolNetBid            = nmtypes.MustIntern("depthflow/net_displayed_flow_bid")
	symbolNetAsk            = nmtypes.MustIntern("depthflow/net_displayed_flow_ask")
	symbolNegNetBid         = nmtypes.MustIntern("depthflow/neg_net_bid")
	symbolNegNetAsk         = nmtypes.MustIntern("depthflow/neg_net_ask")
	symbolAddedBid          = nmtypes.MustIntern("depthflow/added_notional_bid")
	symbolRemovedBid        = nmtypes.MustIntern("depthflow/removed_notional_bid")
	symbolAddedAsk          = nmtypes.MustIntern("depthflow/added_notional_ask")
	symbolRemovedAsk        = nmtypes.MustIntern("depthflow/removed_notional_ask")
	symbolMutationBid       = nmtypes.MustIntern("depthflow/mutation_bid")
	symbolMutationAsk       = nmtypes.MustIntern("depthflow/mutation_ask")
	symbolMutation          = nmtypes.MustIntern("depthflow/mutation")
	symbolPrevBookNotional  = nmtypes.MustIntern("depthflow/prev_book_notional")
	symbolReferenceDepth    = nmtypes.MustIntern("depthflow/reference_depth")
	symbolBookNotionalDiff  = nmtypes.MustIntern("depthflow/book_notional_diff")
	symbolNetDiff           = nmtypes.MustIntern("depthflow/net_diff")
	symbolScaleDenominator  = nmtypes.MustIntern("depthflow/scale_denominator")
	symbolNetBidRate        = nmtypes.MustIntern("depthflow/net_displayed_flow_rate_bid")
	symbolNetAskRate        = nmtypes.MustIntern("depthflow/net_displayed_flow_rate_ask")
	symbolAddedBidRate      = nmtypes.MustIntern("depthflow/added_notional_rate_bid")
	symbolRemovedBidRate    = nmtypes.MustIntern("depthflow/removed_notional_rate_bid")
	symbolAddedAskRate      = nmtypes.MustIntern("depthflow/added_notional_rate_ask")
	symbolRemovedAskRate    = nmtypes.MustIntern("depthflow/removed_notional_rate_ask")
	symbolTurnoverRate      = nmtypes.MustIntern("depthflow/book_turnover_rate")
	symbolNetBookChangeRate = nmtypes.MustIntern("depthflow/net_book_change_rate")
	symbolSignedNetFlowRate = nmtypes.MustIntern("depthflow/signed_net_displayed_flow_rate")
	symbolActivityDiff      = nmtypes.MustIntern("depthflow/activity_diff")
	symbolFlowActivityImbal = nmtypes.MustIntern("depthflow/flow_activity_imbalance")

	// Estimator chain slot tables (namespaced, one per measured quantity).
	bookImbalanceSlots = newEstimatorSlots("book_imbalance")
	resolutionGapSlots = newEstimatorSlots("resolution_gap")
	turnoverSlots      = newEstimatorSlots("turnover")

	// Velocity outputs and log-space turnover intermediates.
	symbolBookImbalanceVelocity = nmtypes.MustIntern("depthflow/book_imbalance_velocity")
	symbolResolutionGapVelocity = nmtypes.MustIntern("depthflow/resolution_gap_velocity")
	symbolLogTurnover           = nmtypes.MustIntern("depthflow/log_turnover")
	symbolTurnoverBaseline      = nmtypes.MustIntern("depthflow/turnover_baseline")
	symbolTurnoverRatio         = nmtypes.MustIntern("depthflow/turnover_ratio")
	symbolTurnoverVelocity      = nmtypes.MustIntern("depthflow/turnover_velocity")

	symbolZero          = nmtypes.MustIntern("depthflow/zero")
	symbolDivergence    = nmtypes.MustIntern("divergence")
	symbolNoiseVariance = nmtypes.MustIntern("noise_variance")
)

/*
Rate predicates gate the per-second and scale-free flow metrics so they are
absent rather than divided by zero on the first observation, when no previous
comparable book exists (the elapsed interval is zero).
*/
var (
	deltaPositive = nmtypes.Wire(
		logic.GreaterThan,
		nmtypes.In(temporal.SymbolDelta, calculus.PortA),
		nmtypes.In(symbolZero, calculus.PortB),
		nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
	)
	mutationPositive = nmtypes.Wire(
		logic.GreaterThan,
		nmtypes.In(symbolMutation, calculus.PortA),
		nmtypes.In(symbolZero, calculus.PortB),
		nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
	)
	turnoverPositive = nmtypes.Wire(
		logic.GreaterThan,
		nmtypes.In(symbolTurnoverRate, calculus.PortA),
		nmtypes.In(symbolZero, calculus.PortB),
		nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
	)
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
additiveEstimator routes one bounded additive quantity into its namespaced
series and runs the causal ZScore → Baseline chain. ZScore evaluates the
current observation against the previous step's baseline before Baseline adapts,
per the global causality contract. The ZScore readiness is captured into a
dedicated slot so downstream stages can gate divergence metadata on it.
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
Level3 is the full-book market entity. It owns exactly one Number pipeline and
one projector, both declared in its constructor, plus Step and Close. The book
itself lives in the workspace shared-object pool; this signal never reconstructs
it.
*/
type Level3 struct {
	number    *nomagique.Number[string]
	projector *data.Projector
	workspace *runtime.Workspace
}

/*
NewLevel3 constructs the Level3 entity: one Number pipeline for the depth-flow
metric computation and one projector that names the output slots.
*/
func NewLevel3(workspace *runtime.Workspace) *Level3 {
	return &Level3{
		workspace: workspace,
		number: nomagique.NewNumber[string](nmtypes.Pipe(
			nmtypes.Assign(symbolZero, 0),
			// 0 < touch bid < touch ask: a crossed book is rejected here.
			logic.PositiveOrder(symbolTouchBidPrice, symbolTouchAskPrice),
			// Elapsed interval between this observation and the previous one.
			temporal.Duration,

			// Static book metrics.
			nmtypes.Wire(
				calculus.Sum,
				nmtypes.In(symbolBookNotionalBid, calculus.PortA),
				nmtypes.In(symbolBookNotionalAsk, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolBookNotional),
			),
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(symbolBookNotionalBid, calculus.PortA),
				nmtypes.In(symbolBookNotionalAsk, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolBookDiff),
			),
			nmtypes.Wire(
				calculus.Quotient,
				nmtypes.In(symbolBookDiff, calculus.PortA),
				nmtypes.In(symbolBookNotional, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolBookImbalance),
			),
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(symbolTouchNotionalBid, calculus.PortA),
				nmtypes.In(symbolTouchNotionalAsk, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolTouchDiff),
			),
			nmtypes.Wire(
				calculus.Sum,
				nmtypes.In(symbolTouchNotionalBid, calculus.PortA),
				nmtypes.In(symbolTouchNotionalAsk, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolTouchNotional),
			),
			nmtypes.Wire(
				calculus.Quotient,
				nmtypes.In(symbolTouchDiff, calculus.PortA),
				nmtypes.In(symbolTouchNotional, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolTouchImbalance),
			),
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(symbolTouchImbalance, calculus.PortA),
				nmtypes.In(symbolBookImbalance, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolResolutionGap),
			),
			nmtypes.Wire(
				calculus.Absolute,
				nmtypes.In(symbolResolutionGap, calculus.PortX),
				nmtypes.Out(calculus.PortResult, symbolResolutionDist),
			),

			// Displayed flow metrics: net change, then its positive/negative
			// decomposition into additions and removals per side.
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(symbolBookNotionalBid, calculus.PortA),
				nmtypes.In(symbolPrevBidNotional, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolNetBid),
			),
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(symbolBookNotionalAsk, calculus.PortA),
				nmtypes.In(symbolPrevAskNotional, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolNetAsk),
			),
			nmtypes.Wire(
				calculus.Positive,
				nmtypes.In(symbolNetBid, calculus.PortX),
				nmtypes.Out(calculus.PortResult, symbolAddedBid),
			),
			nmtypes.Wire(
				calculus.Negative,
				nmtypes.In(symbolNetBid, calculus.PortX),
				nmtypes.Out(calculus.PortResult, symbolNegNetBid),
			),
			nmtypes.Wire(
				calculus.Positive,
				nmtypes.In(symbolNegNetBid, calculus.PortX),
				nmtypes.Out(calculus.PortResult, symbolRemovedBid),
			),
			nmtypes.Wire(
				calculus.Positive,
				nmtypes.In(symbolNetAsk, calculus.PortX),
				nmtypes.Out(calculus.PortResult, symbolAddedAsk),
			),
			nmtypes.Wire(
				calculus.Negative,
				nmtypes.In(symbolNetAsk, calculus.PortX),
				nmtypes.Out(calculus.PortResult, symbolNegNetAsk),
			),
			nmtypes.Wire(
				calculus.Positive,
				nmtypes.In(symbolNegNetAsk, calculus.PortX),
				nmtypes.Out(calculus.PortResult, symbolRemovedAsk),
			),

			// Scale-free intermediates.
			nmtypes.Wire(
				calculus.Sum,
				nmtypes.In(symbolAddedBid, calculus.PortA),
				nmtypes.In(symbolRemovedBid, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolMutationBid),
			),
			nmtypes.Wire(
				calculus.Sum,
				nmtypes.In(symbolAddedAsk, calculus.PortA),
				nmtypes.In(symbolRemovedAsk, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolMutationAsk),
			),
			nmtypes.Wire(
				calculus.Sum,
				nmtypes.In(symbolMutationBid, calculus.PortA),
				nmtypes.In(symbolMutationAsk, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolMutation),
			),
			nmtypes.Wire(
				calculus.Sum,
				nmtypes.In(symbolPrevBidNotional, calculus.PortA),
				nmtypes.In(symbolPrevAskNotional, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolPrevBookNotional),
			),
			nmtypes.Wire(
				calculus.Average,
				nmtypes.In(symbolBookNotional, calculus.PortA),
				nmtypes.In(symbolPrevBookNotional, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolReferenceDepth),
			),
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(symbolBookNotional, calculus.PortA),
				nmtypes.In(symbolPrevBookNotional, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolBookNotionalDiff),
			),
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(symbolNetBid, calculus.PortA),
				nmtypes.In(symbolNetAsk, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolNetDiff),
			),
			nmtypes.Wire(
				calculus.Product,
				nmtypes.In(symbolReferenceDepth, calculus.PortA),
				nmtypes.In(temporal.SymbolDelta, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolScaleDenominator),
			),

			// Book imbalance and resolution gap estimator chains, plus their
			// velocities.
			additiveEstimator(bookImbalanceSlots, symbolBookImbalance),
			// Surface the book-imbalance estimator support as the measurement's
			// sample count so Finalize derives Maturity (1 - 1/support).
			nmtypes.Wire(
				nmtypes.Identity,
				nmtypes.In(bookImbalanceSlots.series.CountSymbol, nmtypes.SampleCount),
				nmtypes.Out(nmtypes.SampleCount, nmtypes.SampleCount),
			),
			velocityChain("book_imbalance_velocity", symbolBookImbalance, symbolBookImbalanceVelocity),
			additiveEstimator(resolutionGapSlots, symbolResolutionGap),
			velocityChain("resolution_gap_velocity", symbolResolutionGap, symbolResolutionGapVelocity),

			// Per-second and scale-free rates, gated on a positive interval.
			logic.If(
				deltaPositive,
				nmtypes.Pipe(
					nmtypes.Wire(
						calculus.Quotient,
						nmtypes.In(symbolNetBid, calculus.PortA),
						nmtypes.In(temporal.SymbolDelta, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolNetBidRate),
					),
					nmtypes.Wire(
						calculus.Quotient,
						nmtypes.In(symbolNetAsk, calculus.PortA),
						nmtypes.In(temporal.SymbolDelta, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolNetAskRate),
					),
					nmtypes.Wire(
						calculus.Quotient,
						nmtypes.In(symbolAddedBid, calculus.PortA),
						nmtypes.In(temporal.SymbolDelta, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolAddedBidRate),
					),
					nmtypes.Wire(
						calculus.Quotient,
						nmtypes.In(symbolRemovedBid, calculus.PortA),
						nmtypes.In(temporal.SymbolDelta, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolRemovedBidRate),
					),
					nmtypes.Wire(
						calculus.Quotient,
						nmtypes.In(symbolAddedAsk, calculus.PortA),
						nmtypes.In(temporal.SymbolDelta, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolAddedAskRate),
					),
					nmtypes.Wire(
						calculus.Quotient,
						nmtypes.In(symbolRemovedAsk, calculus.PortA),
						nmtypes.In(temporal.SymbolDelta, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolRemovedAskRate),
					),
					nmtypes.Wire(
						calculus.Quotient,
						nmtypes.In(symbolMutation, calculus.PortA),
						nmtypes.In(symbolScaleDenominator, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolTurnoverRate),
					),
					nmtypes.Wire(
						calculus.Quotient,
						nmtypes.In(symbolBookNotionalDiff, calculus.PortA),
						nmtypes.In(symbolScaleDenominator, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolNetBookChangeRate),
					),
					nmtypes.Wire(
						calculus.Quotient,
						nmtypes.In(symbolNetDiff, calculus.PortA),
						nmtypes.In(symbolScaleDenominator, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolSignedNetFlowRate),
					),
				),
				nil,
			),

			// Flow activity imbalance, gated on positive gross mutation.
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(symbolMutationBid, calculus.PortA),
				nmtypes.In(symbolMutationAsk, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolActivityDiff),
			),
			logic.If(
				mutationPositive,
				nmtypes.Wire(
					calculus.Quotient,
					nmtypes.In(symbolActivityDiff, calculus.PortA),
					nmtypes.In(symbolMutation, calculus.PortB),
					nmtypes.Out(calculus.PortResult, symbolFlowActivityImbal),
				),
				nil,
			),

			// Log-space turnover estimator chain, gated on a positive turnover
			// rate (the zero-turnover state is retained, not logged).
			logic.If(
				deltaPositive,
				logic.If(
					turnoverPositive,
					nmtypes.Pipe(
						logEstimator(
							turnoverSlots,
							symbolTurnoverRate,
							symbolLogTurnover,
							symbolTurnoverBaseline,
							symbolTurnoverRatio,
						),
						velocityChain("turnover_velocity", symbolLogTurnover, symbolTurnoverVelocity),
					),
					nil,
				),
				nil,
			),

			// Estimator quality facts for data.Measurement.Finalize, gated on
			// the book-imbalance z-score readiness.
			logic.If(
				nmtypes.Wire(
					nmtypes.Identity,
					nmtypes.In(bookImbalanceSlots.ready, logic.SymbolCondition),
					nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
				),
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
			data.Binding{From: symbolBookNotionalBid, Name: "book_notional:bid", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBookNotionalAsk, Name: "book_notional:ask", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBookNotional, Name: "book_notional", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBookImbalance, Name: "book_imbalance", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolTouchImbalance, Name: "touch_imbalance", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolResolutionGap, Name: "imbalance_resolution_gap", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolResolutionDist, Name: "imbalance_resolution_distance", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolNetBid, Name: "net_displayed_flow:bid", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolNetAsk, Name: "net_displayed_flow:ask", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAddedBid, Name: "added_notional:bid", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolRemovedBid, Name: "removed_notional:bid", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAddedAsk, Name: "added_notional:ask", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolRemovedAsk, Name: "removed_notional:ask", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolNetBidRate, Name: "net_displayed_flow_rate:bid", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolNetAskRate, Name: "net_displayed_flow_rate:ask", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolAddedBidRate, Name: "added_notional_rate:bid", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolRemovedBidRate, Name: "removed_notional_rate:bid", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolAddedAskRate, Name: "added_notional_rate:ask", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolRemovedAskRate, Name: "removed_notional_rate:ask", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolTurnoverRate, Name: "book_turnover_rate", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolNetBookChangeRate, Name: "net_book_change_rate", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolSignedNetFlowRate, Name: "signed_net_displayed_flow_rate", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolFlowActivityImbal, Name: "flow_activity_imbalance", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: bookImbalanceSlots.baseline, Name: "book_imbalance_baseline", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: bookImbalanceSlots.residual, Name: "book_imbalance_divergence", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: bookImbalanceSlots.zscore, Name: "book_imbalance_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolBookImbalanceVelocity, Name: "book_imbalance_velocity", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: resolutionGapSlots.baseline, Name: "resolution_gap_baseline", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: resolutionGapSlots.residual, Name: "resolution_gap_divergence", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: resolutionGapSlots.zscore, Name: "resolution_gap_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolResolutionGapVelocity, Name: "resolution_gap_velocity", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
			data.Binding{From: symbolTurnoverBaseline, Name: "turnover_baseline", Unit: data.UnitPerSecond, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolTurnoverRatio, Name: "turnover_ratio", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: turnoverSlots.zscore, Name: "turnover_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolTurnoverVelocity, Name: "turnover_velocity", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
		),
	}
}

/*
Step reads the shared book for one symbol, aggregates the displayed depth
facts, runs the Number pipeline, and projects exactly one Measurement. A missing
or type-mismatched book yields no measurement; the caller skips it rather than
panicking.
*/
func (level3 *Level3) Step(symbol string, at time.Time) *data.Measurement[float64] {
	shared, found := level3.workspace.Shared("book", symbol)

	if !found {
		return nil
	}

	orderBook, ok := shared.(*book.Book)

	if !ok {
		return nil
	}

	bidNotional := 0.0

	for _, priceLevel := range orderBook.Bids.Levels {
		bidNotional += priceLevel.Price.Float64() * priceLevel.Quantity.Float64()
	}

	askNotional := 0.0

	for _, priceLevel := range orderBook.Asks.Levels {
		askNotional += priceLevel.Price.Float64() * priceLevel.Quantity.Float64()
	}

	touchBid := orderBook.BestBid()
	touchAsk := orderBook.BestAsk()

	if touchBid == nil || touchAsk == nil {
		return nil
	}

	touchBidPrice := touchBid.Price.Float64()
	touchAskPrice := touchAsk.Price.Float64()
	touchBidNotional := touchBidPrice * touchBid.Quantity.Float64()
	touchAskNotional := touchAskPrice * touchAsk.Quantity.Float64()

	previousBidNotional := bidNotional
	previousAskNotional := askNotional
	previousSec := float64(at.Unix())
	previousNsec := float64(at.Nanosecond())

	if committed, found := level3.number.Project(symbol); found {
		previousBidNotional, _ = committed.Get(symbolBookNotionalBid)
		previousAskNotional, _ = committed.Get(symbolBookNotionalAsk)
		previousSec, _ = committed.Get(nmtypes.EventTimeSec)
		previousNsec, _ = committed.Get(nmtypes.EventTimeNsec)
	}

	input := nmtypes.Frame{}
	input.Put(symbolBookNotionalBid, bidNotional)
	input.Put(symbolBookNotionalAsk, askNotional)
	input.Put(symbolPrevBidNotional, previousBidNotional)
	input.Put(symbolPrevAskNotional, previousAskNotional)
	input.Put(symbolTouchBidPrice, touchBidPrice)
	input.Put(symbolTouchAskPrice, touchAskPrice)
	input.Put(symbolTouchNotionalBid, touchBidNotional)
	input.Put(symbolTouchNotionalAsk, touchAskNotional)
	input.Put(nmtypes.EventTimeSec, float64(at.Unix()))
	input.Put(nmtypes.EventTimeNsec, float64(at.Nanosecond()))
	input.Put(temporal.SymbolCurrentSec, float64(at.Unix()))
	input.Put(temporal.SymbolCurrentNsec, float64(at.Nanosecond()))
	input.Put(temporal.SymbolPreviousSec, previousSec)
	input.Put(temporal.SymbolPreviousNsec, previousNsec)

	return level3.projector.Project(
		symbol,
		"depthflow",
		at,
		at,
		level3.number.Step(symbol, input),
	)
}

func (level3 *Level3) Close() error { return nil }
