package pumpdump

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
Input/output slot symbols for the executable-spread metric pipeline.
*/
var (
	symbolBidPrice       = nmtypes.MustIntern("pumpdump/bid_price")
	symbolAskPrice       = nmtypes.MustIntern("pumpdump/ask_price")
	symbolMidpoint       = nmtypes.MustIntern("pumpdump/midpoint")
	symbolSpread         = nmtypes.MustIntern("pumpdump/spread")
	symbolRelativeSpread = nmtypes.MustIntern("pumpdump/relative_spread")
	symbolLogRelative    = nmtypes.MustIntern("pumpdump/log_relative_spread")
	symbolNegBaseline    = nmtypes.MustIntern("pumpdump/neg_baseline")
	symbolExpBaseline    = nmtypes.MustIntern("pumpdump/relative_spread_baseline")
	symbolSpreadRatio    = nmtypes.MustIntern("pumpdump/spread_ratio")
	symbolDivergence     = nmtypes.MustIntern("divergence")
	symbolNoiseVariance  = nmtypes.MustIntern("noise_variance")
	symbolOne            = nmtypes.MustIntern("pumpdump/one")
)

/*
Ticker is the executable-touch market entity. It owns exactly a Number pipeline
and a projector, both declared in its constructor, plus Step and Close.
*/
type Ticker struct {
	number    *nomagique.Number[string]
	projector *data.Projector
}

/*
NewTicker constructs the Ticker entity: one Number pipeline for the touch and
spread historical context, and one projector that names the output slots.
*/
func NewTicker() *Ticker {
	return &Ticker{
		number: nomagique.NewNumber[string](nmtypes.Pipe(
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
			nmtypes.Wire(
				nmtypes.Identity,
				nmtypes.In(symbolLogRelative, calculus.PortX),
				nmtypes.Out(calculus.PortX, nmtypes.SampleValue),
			),
			nmtypes.Configure(
				statistic.Baseline(""),
				nmtypes.Span,
				temporal.Window(""),
			),
			statistic.ZScore(""),
			// Relative spread baseline: exp of the log baseline (e^mu).
			nmtypes.Wire(
				calculus.Negative,
				nmtypes.In(statistic.SymbolBaselineValue, calculus.PortX),
				nmtypes.Out(calculus.PortResult, symbolNegBaseline),
			),
			nmtypes.Wire(
				calculus.Exponential,
				nmtypes.In(symbolNegBaseline, calculus.SymbolProgress),
				nmtypes.Out(calculus.PortResult, symbolExpBaseline),
			),
			// Spread ratio: relative_spread / relative_spread_baseline
			nmtypes.Wire(
				calculus.Quotient,
				nmtypes.In(symbolRelativeSpread, calculus.PortA),
				nmtypes.In(symbolExpBaseline, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolSpreadRatio),
			),
			// Carry the spread divergence and noise power so Finalize derives SNR.
			nmtypes.Wire(
				nmtypes.Identity,
				nmtypes.In(statistic.SymbolResidual, calculus.PortX),
				nmtypes.Out(calculus.PortX, symbolDivergence),
			),
			nmtypes.Wire(
				calculus.Product,
				nmtypes.In(statistic.SymbolDispersion, calculus.PortA),
				nmtypes.In(statistic.SymbolDispersion, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolNoiseVariance),
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
		),
	}
}

/*
Step receives one ticker data point, loads the touch facts, runs the Number
pipeline, and projects exactly one Measurement.
*/
func (ticker *Ticker) Step(point kraken.TickerData) *data.Measurement[float64] {
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
