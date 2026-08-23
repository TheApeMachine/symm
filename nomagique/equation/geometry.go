package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolCenter        = nmtypes.MustIntern("equation/center")
	SymbolWidth         = nmtypes.MustIntern("equation/width")
	SymbolRelativeWidth = nmtypes.MustIntern("equation/relative_width")
	SymbolDissimilarity = nmtypes.MustIntern("equation/dissimilarity")
	SymbolBalance       = nmtypes.MustIntern("equation/balance")
	SymbolCompression   = nmtypes.MustIntern("equation/compression")
	SymbolDeviation     = nmtypes.MustIntern("equation/deviation")
)

/*
Geometry conditions two positive price and quantity channels. It emits their
center, width, log quantity balance, relative width, and causal compression.
*/
func Geometry() nmtypes.Primitive {
	return nmtypes.Pipe(
		logic.Observe(
			nmtypes.AlphaPrice,
			nmtypes.BetaPrice,
			nmtypes.AlphaQuantity,
			nmtypes.BetaQuantity,
			nmtypes.EventTimeSec,
			nmtypes.EventTimeNsec,
		),
		logic.PositiveOrder(nmtypes.AlphaPrice, nmtypes.BetaPrice),
		nmtypes.Fork(channelCenter(), channelBalance()),
		channelWidth(),
		nmtypes.Relay(SymbolWidth, nmtypes.SampleValue),
		CausalBaseline(),
		statistic.Maturity(nmtypes.SampleCount),
		nmtypes.Fork(
			relativeWidth(),
			nmtypes.Fork(
				dissimilarity(),
				nmtypes.Fork(deviation(), compression()),
			),
		),
	)
}

func deviation() nmtypes.Primitive {
	return logic.If(
		nmtypes.Relay(statistic.SymbolReady, logic.SymbolCondition),
		nmtypes.Pipe(
			nmtypes.Relay(SymbolWidth, calculus.SymbolLeft),
			nmtypes.Relay(statistic.SymbolMean, calculus.SymbolRight),
			calculus.Difference,
			nmtypes.Relay(calculus.SymbolResult, SymbolDeviation),
		),
		nmtypes.Identity,
	)
}

func channelCenter() nmtypes.Primitive {
	return nmtypes.Pipe(
		nmtypes.Relay(nmtypes.AlphaPrice, calculus.SymbolLeft),
		nmtypes.Relay(nmtypes.BetaPrice, calculus.SymbolRight),
		calculus.Average,
		nmtypes.Relay(calculus.SymbolResult, SymbolCenter),
	)
}

func channelBalance() nmtypes.Primitive {
	return nmtypes.Pipe(
		nmtypes.Relay(nmtypes.AlphaQuantity, calculus.SymbolCurrent),
		nmtypes.Relay(nmtypes.BetaQuantity, calculus.SymbolPrevious),
		calculus.LogRatio,
		nmtypes.Relay(calculus.SymbolResult, SymbolBalance),
	)
}

func channelWidth() nmtypes.Primitive {
	return nmtypes.Pipe(
		nmtypes.Relay(nmtypes.BetaPrice, calculus.SymbolLeft),
		nmtypes.Relay(nmtypes.AlphaPrice, calculus.SymbolRight),
		calculus.Difference,
		nmtypes.Relay(calculus.SymbolResult, SymbolWidth),
	)
}

func relativeWidth() nmtypes.Primitive {
	return nmtypes.Pipe(
		nmtypes.Relay(SymbolWidth, calculus.SymbolValue),
		nmtypes.Relay(SymbolCenter, calculus.SymbolBaseline),
		nmtypes.Assign(calculus.SymbolReady, 1),
		calculus.Ratio,
		nmtypes.Relay(calculus.SymbolResult, SymbolRelativeWidth),
	)
}

func dissimilarity() nmtypes.Primitive {
	return nmtypes.Pipe(
		nmtypes.Relay(nmtypes.AlphaPrice, calculus.SymbolLeft),
		nmtypes.Relay(nmtypes.BetaPrice, calculus.SymbolRight),
		calculus.Sum,
		nmtypes.Relay(calculus.SymbolResult, calculus.SymbolRight),
		nmtypes.Relay(SymbolWidth, calculus.SymbolLeft),
		calculus.Quotient,
		nmtypes.Relay(calculus.SymbolResult, SymbolDissimilarity),
	)
}

func compression() nmtypes.Primitive {
	return logic.If(
		nmtypes.Relay(statistic.SymbolReady, logic.SymbolCondition),
		nmtypes.Pipe(
			nmtypes.Relay(statistic.SymbolMean, calculus.SymbolLeft),
			nmtypes.Relay(SymbolWidth, calculus.SymbolRight),
			calculus.Difference,
			nmtypes.Relay(calculus.SymbolResult, calculus.SymbolValue),
			calculus.Positive,
			nmtypes.Relay(calculus.SymbolResult, calculus.SymbolValue),
			nmtypes.Relay(statistic.SymbolMean, calculus.SymbolBaseline),
			calculus.Ratio,
			nmtypes.Relay(calculus.SymbolResult, SymbolCompression),
		),
		nmtypes.Identity,
	)
}
