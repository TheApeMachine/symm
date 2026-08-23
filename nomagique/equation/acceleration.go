package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolClosed = nmtypes.MustIntern("equation/closed")
	SymbolTarget = nmtypes.MustIntern("equation/target")
)

/*
Acceleration is a quantity-clocked rate equation. A data-derived median sizes
each accumulation span; event time supplies its duration; and the configured
alpha price is observed only at completed spans to expose a causal log change.
*/
func Acceleration() nmtypes.Primitive {
	return nmtypes.Pipe(
		logic.Observe(
			nmtypes.Quantity,
			nmtypes.AlphaPrice,
			nmtypes.EventTimeSec,
			nmtypes.EventTimeNsec,
		),
		temporal.Window,
		statistic.Median,
		nmtypes.Relay(statistic.SymbolResult, SymbolTarget),
		nmtypes.Relay(SymbolTarget, calculus.SymbolBaseline),
		temporal.Since,
		nmtypes.Relay(nmtypes.Quantity, calculus.SymbolDelta),
		calculus.Accumulate,
		logic.If(accelerationClosed(), closeAcceleration(), openAcceleration()),
	)
}

func accelerationClosed() nmtypes.Primitive {
	return nmtypes.Pipe(
		nmtypes.Fork(
			nmtypes.Pipe(
				nmtypes.Relay(calculus.SymbolTotal, calculus.SymbolLeft),
				nmtypes.Relay(calculus.SymbolBaseline, calculus.SymbolRight),
				logic.GreaterOrEqual,
				nmtypes.Relay(logic.SymbolCondition, calculus.SymbolLeft),
			),
			nmtypes.Relay(temporal.SymbolAdvanced, calculus.SymbolRight),
		),
		logic.And,
	)
}

func closeAcceleration() nmtypes.Primitive {
	return nmtypes.Pipe(
		nmtypes.Relay(calculus.SymbolTotal, calculus.SymbolCount),
		calculus.Rate,
		temporal.Observer(nmtypes.AlphaPrice),
		logic.If(
			nmtypes.Relay(calculus.SymbolReady, logic.SymbolCondition),
			nmtypes.Pipe(
				calculus.LogRatio,
				nmtypes.Relay(calculus.SymbolResult, SymbolChange),
			),
			nmtypes.Identity,
		),
		temporal.Restart,
		calculus.Clear(calculus.SymbolTotal),
		nmtypes.Assign(SymbolClosed, 1),
		statistic.Maturity(temporal.SymbolCompletedSpans),
	)
}

func openAcceleration() nmtypes.Primitive {
	return nmtypes.Pipe(
		nmtypes.Assign(SymbolClosed, 0),
		statistic.Maturity(temporal.SymbolCompletedSpans),
	)
}
