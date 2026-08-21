package equation

import (
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
)

var SymbolRelativeChange = nomagique.MustIntern("equation/relative_change")

/*
RelativeChange reports (current-previous)/previous for a positive observed
series. The first observation remains provisional and event-time regression
is rejected by Observer.
*/
func RelativeChange(source nomagique.Symbol) nomagique.Primitive {
	return nomagique.Pipe(
		temporal.Observer(source),
		statistic.Maturity(temporal.SymbolObservations),
		logic.If(
			nomagique.Relay(calculus.SymbolReady, logic.SymbolCondition),
			nomagique.Pipe(
				nomagique.Relay(calculus.SymbolCurrent, calculus.SymbolLeft),
				nomagique.Relay(calculus.SymbolPrevious, calculus.SymbolRight),
				calculus.Difference,
				nomagique.Relay(calculus.SymbolResult, SymbolChange),
				nomagique.Relay(SymbolChange, calculus.SymbolLeft),
				nomagique.Relay(calculus.SymbolPrevious, calculus.SymbolRight),
				calculus.Quotient,
				nomagique.Relay(calculus.SymbolResult, SymbolRelativeChange),
			),
			nomagique.Identity,
		),
	)
}
