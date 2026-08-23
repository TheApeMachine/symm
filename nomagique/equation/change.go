package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

var SymbolChange = types.MustIntern("equation/change")

/*
Change observes one named fact, then explicitly binds the causal pair to the
local A/B ports of Difference. The first observation emits no invented change.
*/
func Change(source types.Symbol) types.Primitive {
	return types.Pipe(
		temporal.Observer(source),
		types.ForkStrict(
			statistic.Maturity(temporal.SymbolObservations),
			logic.If(
				types.Wire(
					types.Identity,
					types.In(calculus.SymbolReady, logic.SymbolCondition),
					types.Out(logic.SymbolCondition, logic.SymbolCondition),
				),
				types.Wire(
					calculus.Difference,
					types.In(calculus.SymbolCurrent, calculus.PortA),
					types.In(calculus.SymbolPrevious, calculus.PortB),
					types.Out(calculus.PortResult, SymbolChange),
				),
				types.Identity,
			),
		),
	)
}
