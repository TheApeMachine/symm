package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolAlpha           = types.MustIntern("equation/alpha")
	SymbolBeta            = types.MustIntern("equation/beta")
	SymbolAlphaNormalized = types.MustIntern("equation/alpha_normalized")
	SymbolBetaNormalized  = types.MustIntern("equation/beta_normalized")
	symbolNegatedChange   = types.MustIntern("equation/negated_change")
)

// Polarize decomposes a signed change and explicitly normalizes both components.
func Polarize() types.Primitive {
	return types.Pipe(
		logic.EnsureFinite(SymbolChange, calculus.SymbolScale, calculus.SymbolReady),
		Decompose(),
		logic.If(
			types.Wire(
				types.Identity,
				types.In(calculus.SymbolReady, logic.SymbolCondition),
				types.Out(logic.SymbolCondition, logic.SymbolCondition),
			),
			types.ForkStrict(
				types.Wire(
					calculus.Squash,
					types.In(SymbolAlpha, calculus.PortX),
					types.In(calculus.SymbolScale, calculus.SymbolScale),
					types.Out(calculus.PortResult, SymbolAlphaNormalized),
				),
				types.Wire(
					calculus.Squash,
					types.In(SymbolBeta, calculus.PortX),
					types.In(calculus.SymbolScale, calculus.SymbolScale),
					types.Out(calculus.PortResult, SymbolBetaNormalized),
				),
			),
			types.Identity,
		),
	)
}

// Decompose splits one signed change into reciprocal non-negative components.
func Decompose() types.Primitive {
	return types.Pipe(
		types.ForkStrict(
			types.Wire(
				calculus.Positive,
				types.In(SymbolChange, calculus.PortX),
				types.Out(calculus.PortResult, SymbolAlpha),
			),
			types.Wire(
				calculus.Negative,
				types.In(SymbolChange, calculus.PortX),
				types.Out(calculus.PortResult, symbolNegatedChange),
			),
		),
		types.Wire(
			calculus.Positive,
			types.In(symbolNegatedChange, calculus.PortX),
			types.Out(calculus.PortResult, SymbolBeta),
		),
	)
}
