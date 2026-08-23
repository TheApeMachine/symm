package equation

import (
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

// LogChange reports log(current/previous) for a positive observed series.
func LogChange(source nomagique.Symbol) types.Primitive {
	return types.Pipe(
		temporal.Observer(source),
		statistic.Maturity(temporal.SymbolObservations),
		logic.If(
			types.Wire(
				types.Identity,
				types.In(calculus.SymbolReady, logic.SymbolCondition),
				types.Out(logic.SymbolCondition, logic.SymbolCondition),
			),
			types.Wire(
				calculus.LogRatio,
				types.In(calculus.SymbolCurrent, calculus.SymbolCurrent),
				types.In(calculus.SymbolPrevious, calculus.SymbolPrevious),
				types.Out(calculus.PortResult, SymbolChange),
			),
			types.Identity,
		),
	)
}
