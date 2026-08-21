package equation

import (
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolCenter        = nomagique.MustIntern("equation/center")
	SymbolWidth         = nomagique.MustIntern("equation/width")
	SymbolRelativeWidth = nomagique.MustIntern("equation/relative_width")
	SymbolDissimilarity = nomagique.MustIntern("equation/dissimilarity")
	SymbolBalance       = nomagique.MustIntern("equation/balance")
	SymbolCompression   = nomagique.MustIntern("equation/compression")
	SymbolDeviation     = nomagique.MustIntern("equation/deviation")
)

/*
Geometry conditions two positive price and quantity channels. It emits their
center, width, log quantity balance, relative width, and causal compression.
*/
func Geometry() nomagique.Primitive {
	return nomagique.Pipe(
		logic.Observe(
			nmtypes.AlphaPrice,
			nmtypes.BetaPrice,
			nmtypes.AlphaQuantity,
			nmtypes.BetaQuantity,
			nmtypes.EventTimeSec,
			nmtypes.EventTimeNsec,
		),
		logic.PositiveOrder(nmtypes.AlphaPrice, nmtypes.BetaPrice),
		nomagique.Fork(channelCenter(), channelBalance()),
		channelWidth(),
		nomagique.Relay(SymbolWidth, nomagique.SampleValue),
		CausalBaseline(),
		statistic.Maturity(nomagique.SampleCount),
		nomagique.Fork(
			relativeWidth(),
			nomagique.Fork(
				dissimilarity(),
				nomagique.Fork(deviation(), compression()),
			),
		),
	)
}

func deviation() nomagique.Primitive {
	return logic.If(
		nomagique.Relay(statistic.SymbolReady, logic.SymbolCondition),
		nomagique.Pipe(
			nomagique.Relay(SymbolWidth, calculus.SymbolLeft),
			nomagique.Relay(statistic.SymbolMean, calculus.SymbolRight),
			calculus.Difference,
			nomagique.Relay(calculus.SymbolResult, SymbolDeviation),
		),
		nomagique.Identity,
	)
}

func channelCenter() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.Relay(nmtypes.AlphaPrice, calculus.SymbolLeft),
		nomagique.Relay(nmtypes.BetaPrice, calculus.SymbolRight),
		calculus.Average,
		nomagique.Relay(calculus.SymbolResult, SymbolCenter),
	)
}

func channelBalance() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.Relay(nmtypes.AlphaQuantity, calculus.SymbolCurrent),
		nomagique.Relay(nmtypes.BetaQuantity, calculus.SymbolPrevious),
		calculus.LogRatio,
		nomagique.Relay(calculus.SymbolResult, SymbolBalance),
	)
}

func channelWidth() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.Relay(nmtypes.BetaPrice, calculus.SymbolLeft),
		nomagique.Relay(nmtypes.AlphaPrice, calculus.SymbolRight),
		calculus.Difference,
		nomagique.Relay(calculus.SymbolResult, SymbolWidth),
	)
}

func relativeWidth() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.Relay(SymbolWidth, calculus.SymbolValue),
		nomagique.Relay(SymbolCenter, calculus.SymbolBaseline),
		nomagique.Assign(calculus.SymbolReady, 1),
		calculus.Ratio,
		nomagique.Relay(calculus.SymbolResult, SymbolRelativeWidth),
	)
}

func dissimilarity() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.Relay(nmtypes.AlphaPrice, calculus.SymbolLeft),
		nomagique.Relay(nmtypes.BetaPrice, calculus.SymbolRight),
		calculus.Sum,
		nomagique.Relay(calculus.SymbolResult, calculus.SymbolRight),
		nomagique.Relay(SymbolWidth, calculus.SymbolLeft),
		calculus.Quotient,
		nomagique.Relay(calculus.SymbolResult, SymbolDissimilarity),
	)
}

func compression() nomagique.Primitive {
	return logic.If(
		nomagique.Relay(statistic.SymbolReady, logic.SymbolCondition),
		nomagique.Pipe(
			nomagique.Relay(statistic.SymbolMean, calculus.SymbolLeft),
			nomagique.Relay(SymbolWidth, calculus.SymbolRight),
			calculus.Difference,
			nomagique.Relay(calculus.SymbolResult, calculus.SymbolValue),
			calculus.Positive,
			nomagique.Relay(calculus.SymbolResult, calculus.SymbolValue),
			nomagique.Relay(statistic.SymbolMean, calculus.SymbolBaseline),
			calculus.Ratio,
			nomagique.Relay(calculus.SymbolResult, SymbolCompression),
		),
		nomagique.Identity,
	)
}
