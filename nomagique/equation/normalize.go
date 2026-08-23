package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolRatio      = types.MustIntern("equation/ratio")
	SymbolNormalized = types.MustIntern("equation/normalized")
	SymbolLift       = types.MustIntern("equation/lift")
)

/*
Normalize scores an observation against its causal adaptive center. Every atom
receives only the facts explicitly bound to its structural ports.
*/
func Normalize() types.Primitive {
	return types.Pipe(
		CausalBaseline(),
		statistic.Maturity(types.SampleCount),
		logic.If(
			types.Wire(
				types.Identity,
				types.In(statistic.SymbolReady, logic.SymbolCondition),
				types.Out(logic.SymbolCondition, logic.SymbolCondition),
			),
			types.ForkStrict(
				types.Wire(
					statistic.Lift,
					types.In(types.SampleValue, types.SampleValue),
					types.In(statistic.SymbolMean, statistic.SymbolBaseline),
					types.Out(statistic.SymbolResult, SymbolLift),
				),
				types.Wire(
					calculus.Quotient,
					types.In(types.SampleValue, calculus.PortA),
					types.In(statistic.SymbolMean, calculus.PortB),
					types.Out(calculus.PortResult, SymbolRatio),
				),
				types.Wire(
					calculus.Squash,
					types.In(types.SampleValue, calculus.PortX),
					types.In(statistic.SymbolMean, calculus.SymbolScale),
					types.Out(calculus.PortResult, SymbolNormalized),
				),
			),
			types.Identity,
		),
	)
}
