package equation

import (
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
)

var SymbolRelativeChange = nomagique.MustIntern("equation/relative_change")

// RelativeChange reports (current-previous)/previous for an observed series.
func RelativeChange(source nomagique.Symbol) nomagique.Primitive {
	return nomagique.Pipe(
		temporal.Observer(source),
		statistic.Maturity(temporal.SymbolObservations),
		logic.If(
			nomagique.Wire(
				nomagique.Identity,
				nomagique.In(calculus.SymbolReady, logic.SymbolCondition),
				nomagique.Out(logic.SymbolCondition, logic.SymbolCondition),
			),
			nomagique.Pipe(
				nomagique.Wire(
					calculus.Difference,
					nomagique.In(calculus.SymbolCurrent, calculus.PortA),
					nomagique.In(calculus.SymbolPrevious, calculus.PortB),
					nomagique.Out(calculus.PortResult, SymbolChange),
				),
				nomagique.Wire(
					calculus.Quotient,
					nomagique.In(SymbolChange, calculus.PortA),
					nomagique.In(calculus.SymbolPrevious, calculus.PortB),
					nomagique.Out(calculus.PortResult, SymbolRelativeChange),
				),
			),
			nomagique.Identity,
		),
	)
}
