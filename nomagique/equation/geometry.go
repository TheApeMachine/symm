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
	SymbolZero          = nmtypes.MustIntern("equation/zero")
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
		nmtypes.Wire(
			nmtypes.Identity,
			nmtypes.In(SymbolWidth, nmtypes.SampleValue),
			nmtypes.Out(nmtypes.SampleValue, nmtypes.SampleValue),
		),
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
		readyCondition(),
		nmtypes.Wire(
			calculus.Difference,
			nmtypes.In(SymbolWidth, calculus.PortA),
			nmtypes.In(statistic.SymbolMean, calculus.PortB),
			nmtypes.Out(calculus.PortResult, SymbolDeviation),
		),
		nmtypes.Identity,
	)
}

func readyCondition() nmtypes.Primitive {
	return nmtypes.Wire(
		nmtypes.Identity,
		nmtypes.In(statistic.SymbolReady, logic.SymbolCondition),
		nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
	)
}

func channelCenter() nmtypes.Primitive {
	return nmtypes.Wire(
		calculus.Average,
		nmtypes.In(nmtypes.AlphaPrice, calculus.PortA),
		nmtypes.In(nmtypes.BetaPrice, calculus.PortB),
		nmtypes.Out(calculus.PortResult, SymbolCenter),
	)
}

func channelBalance() nmtypes.Primitive {
	return nmtypes.Wire(
		calculus.LogRatio,
		nmtypes.In(nmtypes.AlphaQuantity, calculus.SymbolCurrent),
		nmtypes.In(nmtypes.BetaQuantity, calculus.SymbolPrevious),
		nmtypes.Out(calculus.PortResult, SymbolBalance),
	)
}

func channelWidth() nmtypes.Primitive {
	return nmtypes.Wire(
		calculus.Difference,
		nmtypes.In(nmtypes.BetaPrice, calculus.PortA),
		nmtypes.In(nmtypes.AlphaPrice, calculus.PortB),
		nmtypes.Out(calculus.PortResult, SymbolWidth),
	)
}

func relativeWidth() nmtypes.Primitive {
	return nmtypes.Wire(
		calculus.Quotient,
		nmtypes.In(SymbolWidth, calculus.PortA),
		nmtypes.In(SymbolCenter, calculus.PortB),
		nmtypes.Out(calculus.PortResult, SymbolRelativeWidth),
	)
}

func dissimilarity() nmtypes.Primitive {
	return nmtypes.Pipe(
		nmtypes.Wire(
			calculus.Sum,
			nmtypes.In(nmtypes.AlphaPrice, calculus.PortA),
			nmtypes.In(nmtypes.BetaPrice, calculus.PortB),
			nmtypes.Out(calculus.PortResult, calculus.PortB),
		),
		nmtypes.Wire(
			calculus.Quotient,
			nmtypes.In(SymbolWidth, calculus.PortA),
			nmtypes.In(calculus.PortB, calculus.PortB),
			nmtypes.Out(calculus.PortResult, SymbolDissimilarity),
		),
	)
}

func compression() nmtypes.Primitive {
	return logic.If(
		readyCondition(),
		nmtypes.Pipe(
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(statistic.SymbolMean, calculus.PortA),
				nmtypes.In(SymbolWidth, calculus.PortB),
				nmtypes.Out(calculus.PortResult, calculus.PortX),
			),
			nmtypes.Wire(
				calculus.Positive,
				nmtypes.In(calculus.PortX, calculus.PortX),
				nmtypes.Out(calculus.PortResult, calculus.PortX),
			),
			logic.If(
				meanPositive(),
				nmtypes.Wire(
					calculus.Quotient,
					nmtypes.In(calculus.PortX, calculus.PortA),
					nmtypes.In(statistic.SymbolMean, calculus.PortB),
					nmtypes.Out(calculus.PortResult, SymbolCompression),
				),
				nmtypes.Assign(SymbolCompression, 0),
			),
		),
		nmtypes.Identity,
	)
}

func meanPositive() nmtypes.Primitive {
	return nmtypes.Pipe(
		nmtypes.Assign(SymbolZero, 0),
		nmtypes.Wire(
			logic.GreaterThan,
			nmtypes.In(statistic.SymbolMean, calculus.PortA),
			nmtypes.In(SymbolZero, calculus.PortB),
			nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
		),
	)
}
