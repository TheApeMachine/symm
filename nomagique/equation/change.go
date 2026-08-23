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
Change observes one named fact, then explicitly binds the causal pair to the
local A/B ports of Difference. The first observation emits no invented change.
*/
func Change(source nomagique.Symbol) nomagique.Primitive {
	return nomagique.Pipe(
		temporal.Observer(source),
		nomagique.ForkStrict(
			statistic.Maturity(temporal.SymbolObservations),
			logic.If(
				nomagique.Wire(
					nomagique.Identity,
					nomagique.In(calculus.SymbolReady, logic.SymbolCondition),
					nomagique.Out(logic.SymbolCondition, logic.SymbolCondition),
				),
				nomagique.Wire(
					calculus.Difference,
					nomagique.In(calculus.SymbolCurrent, calculus.PortA),
					nomagique.In(calculus.SymbolPrevious, calculus.PortB),
					nomagique.Out(calculus.PortResult, SymbolChange),
				),
				nomagique.Identity,
			),
		),
	)
}
