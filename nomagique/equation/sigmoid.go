package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolSigmoid = nmtypes.MustIntern("equation/sigmoid")
	symbolOne     = nmtypes.MustIntern("equation/one")
)

/*
Sigmoid is the canonical logistic function σ(x) = 1/(1+e^{-x}). It is a pure
composition of atomics: negate x, exponentiate to e^{-x}, add one, and take the
reciprocal quotient.
*/
func Sigmoid() nmtypes.Primitive {
	return nmtypes.Pipe(
		nmtypes.Assign(symbolOne, 1),
		nmtypes.Wire(
			calculus.Exponential,
			nmtypes.In(calculus.PortX, calculus.SymbolProgress),
			nmtypes.Out(calculus.PortResult, calculus.PortX),
		),
		nmtypes.Wire(
			calculus.Sum,
			nmtypes.In(calculus.PortX, calculus.PortA),
			nmtypes.In(symbolOne, calculus.PortB),
			nmtypes.Out(calculus.PortResult, calculus.PortX),
		),
		nmtypes.Wire(
			calculus.Quotient,
			nmtypes.In(symbolOne, calculus.PortA),
			nmtypes.In(calculus.PortX, calculus.PortB),
			nmtypes.Out(calculus.PortResult, SymbolSigmoid),
		),
	)
}
