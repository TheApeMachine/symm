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
Normalize scores a positive observation against its causal adaptive center. It
emits the direct ratio, bounded ratio, lift, and empirical maturity only after
at least one prior observation exists.
*/
func Normalize() nomagique.Primitive {
	return nomagique.Pipe(
		CausalBaseline(),
		statistic.Maturity(nomagique.SampleCount),
		logic.If(
			nomagique.Relay(statistic.SymbolReady, logic.SymbolCondition),
			nomagique.Fork(normalizedLift(), nomagique.Fork(
				normalizedRatio(),
				normalizedScore(),
			)),
			nomagique.Identity,
		),
	)
}

func normalizedLift() nomagique.Primitive {
	return nomagique.Pipe(
		statistic.Lift,
		nomagique.Relay(statistic.SymbolResult, SymbolLift),
	)
}

func normalizedRatio() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.Relay(nomagique.SampleValue, calculus.SymbolValue),
		nomagique.Relay(statistic.SymbolMean, calculus.SymbolBaseline),
		calculus.Ratio,
		nomagique.Relay(calculus.SymbolResult, SymbolRatio),
	)
}

func normalizedScore() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.Relay(nomagique.SampleValue, calculus.SymbolValue),
		nomagique.Relay(statistic.SymbolMean, calculus.SymbolScale),
		calculus.Squash,
		nomagique.Relay(calculus.SymbolResult, SymbolNormalized),
	)
}
