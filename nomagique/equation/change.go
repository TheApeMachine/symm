package equation

import (
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
)

var SymbolChange = nomagique.MustIntern("equation/change")

/*
Change composes a causal Observer with Difference. The first observation has no
invented change; every later output is current minus the immediately preceding
observation and carries empirical maturity.
*/
func Change(source nomagique.Symbol) nomagique.Primitive {
	return nomagique.Pipe(
		temporal.Observer(source),
		nomagique.Fork(
			statistic.Maturity(temporal.SymbolObservations),
			logic.If(
				nomagique.Relay(calculus.SymbolReady, logic.SymbolCondition),
				nomagique.Pipe(
					nomagique.Relay(calculus.SymbolCurrent, calculus.SymbolLeft),
					nomagique.Relay(calculus.SymbolPrevious, calculus.SymbolRight),
					calculus.Difference,
					nomagique.Relay(calculus.SymbolResult, SymbolChange),
				),
				nomagique.Identity,
			),
		),
	)
}
