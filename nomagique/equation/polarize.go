package equation

import (
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
)

var (
	SymbolAlpha           = nomagique.MustIntern("equation/alpha")
	SymbolBeta            = nomagique.MustIntern("equation/beta")
	SymbolAlphaNormalized = nomagique.MustIntern("equation/alpha_normalized")
	SymbolBetaNormalized  = nomagique.MustIntern("equation/beta_normalized")
	symbolNegatedChange   = nomagique.MustIntern("equation/negated_change")
)

// Polarize decomposes a signed change and explicitly normalizes both components.
func Polarize() nomagique.Primitive {
	return nomagique.Pipe(
		logic.EnsureFinite(SymbolChange, calculus.SymbolScale, calculus.SymbolReady),
		Decompose(),
		logic.If(
			nomagique.Wire(
				nomagique.Identity,
				nomagique.In(calculus.SymbolReady, logic.SymbolCondition),
				nomagique.Out(logic.SymbolCondition, logic.SymbolCondition),
			),
			nomagique.ForkStrict(
				nomagique.Wire(
					calculus.Squash,
					nomagique.In(SymbolAlpha, calculus.PortX),
					nomagique.In(calculus.SymbolScale, calculus.SymbolScale),
					nomagique.Out(calculus.PortResult, SymbolAlphaNormalized),
				),
				nomagique.Wire(
					calculus.Squash,
					nomagique.In(SymbolBeta, calculus.PortX),
					nomagique.In(calculus.SymbolScale, calculus.SymbolScale),
					nomagique.Out(calculus.PortResult, SymbolBetaNormalized),
				),
			),
			nomagique.Identity,
		),
	)
}

// Decompose splits one signed change into reciprocal non-negative components.
func Decompose() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.ForkStrict(
			nomagique.Wire(
				calculus.Positive,
				nomagique.In(SymbolChange, calculus.PortX),
				nomagique.Out(calculus.PortResult, SymbolAlpha),
			),
			nomagique.Wire(
				calculus.Negative,
				nomagique.In(SymbolChange, calculus.PortX),
				nomagique.Out(calculus.PortResult, symbolNegatedChange),
			),
		),
		nomagique.Wire(
			calculus.Positive,
			nomagique.In(symbolNegatedChange, calculus.PortX),
			nomagique.Out(calculus.PortResult, SymbolBeta),
		),
	)
}
