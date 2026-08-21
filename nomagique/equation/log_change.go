package equation

import (
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
LogChange reports log(current/previous) for a positive observed series. It is
the scale-invariant counterpart to Change.
*/
func LogChange(source nomagique.Symbol) nomagique.Primitive {
	return nomagique.Pipe(
		temporal.Observer(source),
		statistic.Maturity(temporal.SymbolObservations),
		logic.If(
			nomagique.Relay(calculus.SymbolReady, logic.SymbolCondition),
			nomagique.Pipe(
				calculus.LogRatio,
				nomagique.Relay(calculus.SymbolResult, SymbolChange),
			),
			nomagique.Identity,
		),
	)
}
