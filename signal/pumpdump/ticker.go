package pumpdump

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
Input/output slot symbols for the executable-spread metric pipeline.
*/
var (
	symbolBidPrice       = nmtypes.MustIntern("pumpdump/bid_price")
	symbolAskPrice       = nmtypes.MustIntern("pumpdump/ask_price")
	symbolMidpoint       = nmtypes.MustIntern("pumpdump/midpoint")
	symbolSpread         = nmtypes.MustIntern("pumpdump/spread")
	symbolRelativeSpread = nmtypes.MustIntern("pumpdump/relative_spread")
	symbolLogRelative    = nmtypes.MustIntern("pumpdump/log_relative_spread")
	symbolExpBaseline    = nmtypes.MustIntern("pumpdump/relative_spread_baseline")
	symbolSpreadRatio    = nmtypes.MustIntern("pumpdump/spread_ratio")
	symbolDivergence     = nmtypes.MustIntern("divergence")
	symbolNoiseVariance  = nmtypes.MustIntern("noise_variance")
	symbolOne            = nmtypes.MustIntern("pumpdump/one")
	symbolZero           = nmtypes.MustIntern("pumpdump/zero")
	// symbolTouchComplete marks a Level-3 frame whose bid and ask are BOTH
	// populated (this message's own, or retained from an earlier one).
	symbolTouchComplete = nmtypes.MustIntern("pumpdump/touch_complete")

	// symbolTouchUncrossed marks a Level-3 frame whose completed touch is a
	// real book: 0 < bid < ask. Kraken's Level-3 feed is depth-limited and
	// arrives one side at a time, so a fresh price on one side can transiently
	// sit through the OTHER side's retained price. That frame must still
	// commit its fresh price, or the stale side is kept and every later spread
	// is measured against a price nobody is quoting.
	symbolTouchUncrossed = nmtypes.MustIntern("pumpdump/touch_uncrossed")

	// symbolSurrenderBid/Ask mark a side whose retained touch this message
	// withdrew without naming a replacement. The committed frame is merged
	// UNDER the input, so a surrendered side can only be cleared from inside
	// the pipeline, on the merged frame.
	symbolSurrenderBid = nmtypes.MustIntern("pumpdump/surrender_bid")
	symbolSurrenderAsk = nmtypes.MustIntern("pumpdump/surrender_ask")
)

/*
Ticker is the executable-touch market entity. It owns exactly a Number pipeline
and a projector, both declared in its constructor, plus Step and Close.
*/
type Ticker struct {
	number    *nomagique.KeyedNumber[string]
	projector *data.Projector
}

/*
NewTicker constructs the Ticker entity: one Number pipeline for the touch and
spread historical context, and one projector that names the output slots.
*/
func NewTicker() *Ticker {
	return &Ticker{
		number: nomagique.NewNumber[string](nmtypes.Pipe(
			nmtypes.Assign(symbolZero, 0),
			// 0 < bid < ask: a crossed, missing, or non-positive book is rejected here.
			logic.PositiveOrder(symbolBidPrice, symbolAskPrice),
			// Midpoint: (bid + ask) / 2
			nmtypes.Wire(
				calculus.Average,
				nmtypes.In(symbolBidPrice, calculus.PortA),
				nmtypes.In(symbolAskPrice, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolMidpoint),
			),
			// Spread: ask - bid
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
			nmtypes.Assign(symbolOne, 1),
			// Positive relative spread enters a log baseline: x = log(relative_spread)
			nmtypes.Wire(
				calculus.LogRatio,
				nmtypes.In(symbolRelativeSpread, calculus.SymbolCurrent),
				nmtypes.In(symbolOne, calculus.SymbolPrevious),
				nmtypes.Out(calculus.PortResult, symbolLogRelative),
			),
			// Route the log relative spread through its causal baseline and z-score.
			route(symbolLogRelative, nmtypes.SampleValue),
			statistic.ZScore(""),
			// The spread divergence is the residual; carry it and the noise power
			// so Finalize derives SNR, and measure its first difference.
			logic.If(
				readyCondition(),
				nmtypes.Pipe(
					route(statistic.SymbolResidual, symbolDivergence),
					nmtypes.Wire(
						calculus.Product,
						nmtypes.In(statistic.SymbolDispersion, calculus.PortA),
						nmtypes.In(statistic.SymbolDispersion, calculus.PortB),
						nmtypes.Out(calculus.PortResult, symbolNoiseVariance),
					),
					velocityOver("spread_divergence", statistic.SymbolResidual),
				),
				nmtypes.Identity,
			),
			statistic.Baseline(""),
			temporal.Window(""),
			// Relative spread baseline: exp of the log baseline (e^mu).
			nmtypes.Wire(
				calculus.Exp,
				nmtypes.In(statistic.SymbolBaselineValue, calculus.PortX),
				nmtypes.Out(calculus.PortResult, symbolExpBaseline),
			),
			// Spread ratio: relative_spread / relative_spread_baseline
			nmtypes.Wire(
				calculus.Quotient,
				nmtypes.In(symbolRelativeSpread, calculus.PortA),
				nmtypes.In(symbolExpBaseline, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolSpreadRatio),
			),
		)),
		projector: data.NewProjector(
			data.Binding{From: symbolBidPrice, Name: "best_bid", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskPrice, Name: "best_ask", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolMidpoint, Name: "midpoint", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolSpread, Name: "spread", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolRelativeSpread, Name: "relative_spread", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolExpBaseline, Name: "relative_spread_baseline", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolSpreadRatio, Name: "spread_ratio", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: statistic.SymbolResidual, Name: "spread_divergence", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: statistic.SymbolZScore, Name: "spread_zscore", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: prefixed("spread_divergence", "velocity/delta"), Name: "spread_divergence_velocity", Unit: data.UnitPerSecond, Timescale: data.TimescalePerSecond},
		),
	}
}

/*
Step receives one ticker data point, loads the touch facts, runs the Number
pipeline, and projects exactly one Measurement.
*/
func (ticker *Ticker) Step(point kraken.TickerData) *data.Measurement[float64] {
	if point.Bid == nil || point.Ask == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf("pumpdump: ticker requires bid and ask")}
	}

	if committed, found := ticker.number.Project(point.Symbol); found {
		previousSec, _ := committed.Get(nmtypes.EventTimeSec)
		previousNsec, _ := committed.Get(nmtypes.EventTimeNsec)

		if float64(point.Timestamp.Unix()) < previousSec ||
			(float64(point.Timestamp.Unix()) == previousSec && float64(point.Timestamp.Nanosecond()) < previousNsec) {
			return nil
		}
	}

	input := nmtypes.Frame{}
	input.Put(symbolBidPrice, point.Bid.Float64())
	input.Put(symbolAskPrice, point.Ask.Float64())
	input.Put(nmtypes.EventTimeSec, float64(point.Timestamp.Unix()))
	input.Put(nmtypes.EventTimeNsec, float64(point.Timestamp.Nanosecond()))


	return ticker.projector.Project(
		point.Symbol,
		"pumpdump",
		point.Timestamp,
		point.Timestamp,
		ticker.number.Step(point.Symbol, input),
	)
}

func (ticker *Ticker) Close() error { return nil }

func readyCondition() nmtypes.Primitive {
	return nmtypes.Wire(
		nmtypes.Identity,
		nmtypes.In(calculus.SymbolReady, logic.SymbolCondition),
		nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
	)
}

func seriesReadyCondition(prefix string) nmtypes.Primitive {
	return nmtypes.Wire(
		nmtypes.Identity,
		nmtypes.In(prefixed(prefix, "ready"), logic.SymbolCondition),
		nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
	)
}

func greaterThanCondition(fact nmtypes.Symbol) nmtypes.Primitive {
	return nmtypes.Wire(
		logic.GreaterThan,
		nmtypes.In(fact, calculus.PortA),
		nmtypes.In(symbolZero, calculus.PortB),
		nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
	)
}

func route(from nmtypes.Symbol, to nmtypes.Symbol) nmtypes.Primitive {
	return nmtypes.Wire(
		nmtypes.Identity,
		nmtypes.In(from, calculus.PortX),
		nmtypes.Out(calculus.PortX, to),
	)
}

func prefixed(prefix string, name string) nmtypes.Symbol {
	return nmtypes.MustIntern(temporal.JoinPrefix(prefix, name))
}

/*
velocityOver routes one fact plus the event clock into a namespaced series and
runs the first-difference estimator over it.
*/
func velocityOver(prefix string, value nmtypes.Symbol) nmtypes.Primitive {
	series := temporal.NewSeries(prefix)

	return nmtypes.Pipe(
		route(value, series.ValueSymbol),
		route(nmtypes.EventTimeSec, series.SecSymbol),
		route(nmtypes.EventTimeNsec, series.NsecSymbol),
		statistic.Velocity(prefix),
	)
}

/*
baselineZScore routes one fact plus the event clock into a namespaced series and
runs the causal baseline/z-score chain.
*/
func baselineZScore(prefix string, value nmtypes.Symbol) nmtypes.Primitive {
	series := temporal.NewSeries(prefix)

	return nmtypes.Pipe(
		route(value, series.ValueSymbol),
		route(nmtypes.EventTimeSec, series.SecSymbol),
		route(nmtypes.EventTimeNsec, series.NsecSymbol),
		statistic.ZScore(prefix),
		statistic.Baseline(prefix),
		temporal.Window(prefix),
	)
}
