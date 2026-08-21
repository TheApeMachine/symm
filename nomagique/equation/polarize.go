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
)

/*
Polarize decomposes a signed change into reciprocal non-negative hypotheses and
normalizes both through the supplied positive empirical scale when ready.
*/
func Polarize() nomagique.Primitive {
	return nomagique.Pipe(
		logic.Observe(SymbolChange, calculus.SymbolScale, calculus.SymbolReady),
		Decompose(),
		logic.If(
			nomagique.Relay(calculus.SymbolReady, logic.SymbolCondition),
			nomagique.Fork(alphaNormalization(), betaNormalization()),
			nomagique.Identity,
		),
	)
}

/*
Decompose splits one signed change into reciprocal non-negative alpha and beta
components without choosing a scale or inventing readiness.
*/
func Decompose() nomagique.Primitive {
	return nomagique.Fork(alphaProjection(), betaProjection())
}

func alphaProjection() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.Relay(SymbolChange, calculus.SymbolValue),
		calculus.Positive,
		nomagique.Relay(calculus.SymbolResult, SymbolAlpha),
	)
}

func betaProjection() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.Relay(SymbolChange, calculus.SymbolValue),
		calculus.Negative,
		nomagique.Relay(calculus.SymbolResult, calculus.SymbolValue),
		calculus.Positive,
		nomagique.Relay(calculus.SymbolResult, SymbolBeta),
	)
}

func alphaNormalization() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.Relay(SymbolAlpha, calculus.SymbolValue),
		calculus.Squash,
		nomagique.Relay(calculus.SymbolResult, SymbolAlphaNormalized),
	)
}

func betaNormalization() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.Relay(SymbolBeta, calculus.SymbolValue),
		calculus.Squash,
		nomagique.Relay(calculus.SymbolResult, SymbolBetaNormalized),
	)
}
