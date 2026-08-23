package equation

import (
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

var SymbolRelativeChange = types.MustIntern("equation/relative_change")

// RelativeChange reports (current-previous)/previous for an observed series.
func RelativeChange(source nomagique.Symbol) types.Primitive {
	return types.Pipe(
		temporal.Observer(source),
		statistic.Maturity(temporal.SymbolObservations),
		logic.If(
			types.Wire(
				types.Identity,
				types.In(calculus.SymbolReady, logic.SymbolCondition),
				types.Out(logic.SymbolCondition, logic.SymbolCondition),
			),
			types.Pipe(
				types.Wire(
					calculus.Difference,
					types.In(calculus.SymbolCurrent, calculus.PortA),
					types.In(calculus.SymbolPrevious, calculus.PortB),
					types.Out(calculus.PortResult, SymbolChange),
				),
				types.Wire(
					calculus.Quotient,
					types.In(SymbolChange, calculus.PortA),
					types.In(calculus.SymbolPrevious, calculus.PortB),
					types.Out(calculus.PortResult, SymbolRelativeChange),
				),
			),
			types.Identity,
		),
	)
}
