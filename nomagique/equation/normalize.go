package equation

import (
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
)

var (
	SymbolRatio      = nomagique.MustIntern("equation/ratio")
	SymbolNormalized = nomagique.MustIntern("equation/normalized")
	SymbolLift       = nomagique.MustIntern("equation/lift")
)

/*
Normalize scores an observation against its causal adaptive center. Every atom
receives only the facts explicitly bound to its structural ports.
*/
func Normalize() nomagique.Primitive {
	return nomagique.Pipe(
		CausalBaseline(),
		statistic.Maturity(nomagique.SampleCount),
		logic.If(
			nomagique.Wire(
				nomagique.Identity,
				nomagique.In(statistic.SymbolReady, logic.SymbolCondition),
				nomagique.Out(logic.SymbolCondition, logic.SymbolCondition),
			),
			nomagique.ForkStrict(
				nomagique.Wire(
					statistic.Lift,
					nomagique.In(nomagique.SampleValue, nomagique.SampleValue),
					nomagique.In(statistic.SymbolMean, statistic.SymbolBaseline),
					nomagique.Out(statistic.SymbolResult, SymbolLift),
				),
				nomagique.Wire(
					calculus.Quotient,
					nomagique.In(nomagique.SampleValue, calculus.PortA),
					nomagique.In(statistic.SymbolMean, calculus.PortB),
					nomagique.Out(calculus.PortResult, SymbolRatio),
				),
				nomagique.Wire(
					calculus.Squash,
					nomagique.In(nomagique.SampleValue, calculus.PortX),
					nomagique.In(statistic.SymbolMean, calculus.SymbolScale),
					nomagique.Out(calculus.PortResult, SymbolNormalized),
				),
			),
			nomagique.Identity,
		),
	)
}
