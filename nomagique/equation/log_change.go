package equation

import (
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
)

// LogChange reports log(current/previous) for a positive observed series.
func LogChange(source nomagique.Symbol) nomagique.Primitive {
	return nomagique.Pipe(
		temporal.Observer(source),
		statistic.Maturity(temporal.SymbolObservations),
		logic.If(
			nomagique.Wire(
				nomagique.Identity,
				nomagique.In(calculus.SymbolReady, logic.SymbolCondition),
				nomagique.Out(logic.SymbolCondition, logic.SymbolCondition),
			),
			nomagique.Wire(
				calculus.LogRatio,
				nomagique.In(calculus.SymbolCurrent, calculus.SymbolCurrent),
				nomagique.In(calculus.SymbolPrevious, calculus.SymbolPrevious),
				nomagique.Out(calculus.PortResult, SymbolChange),
			),
			nomagique.Identity,
		),
	)
}
