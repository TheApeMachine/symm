package equation

import (
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolClosed = nomagique.MustIntern("equation/closed")
	SymbolTarget = nomagique.MustIntern("equation/target")
)

/*
Acceleration is a quantity-clocked rate equation. A data-derived median sizes
each accumulation span; event time supplies its duration; and the configured
alpha price is observed only at completed spans to expose a causal log change.
*/
func Acceleration() nomagique.Primitive {
	return nomagique.Pipe(
		logic.Observe(
			nmtypes.Quantity,
			nmtypes.AlphaPrice,
			nmtypes.EventTimeSec,
			nmtypes.EventTimeNsec,
		),
		temporal.Window,
		statistic.Median,
		nomagique.Relay(statistic.SymbolResult, SymbolTarget),
		nomagique.Relay(SymbolTarget, calculus.SymbolBaseline),
		temporal.Since,
		nomagique.Relay(nmtypes.Quantity, calculus.SymbolDelta),
		calculus.Accumulate,
		logic.If(accelerationClosed(), closeAcceleration(), openAcceleration()),
	)
}

func accelerationClosed() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.Fork(
			nomagique.Pipe(
				nomagique.Relay(calculus.SymbolTotal, calculus.SymbolLeft),
				nomagique.Relay(calculus.SymbolBaseline, calculus.SymbolRight),
				logic.GreaterOrEqual,
				nomagique.Relay(logic.SymbolCondition, calculus.SymbolLeft),
			),
			nomagique.Relay(temporal.SymbolAdvanced, calculus.SymbolRight),
		),
		logic.And,
	)
}

func closeAcceleration() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.Relay(calculus.SymbolTotal, calculus.SymbolCount),
		calculus.Rate,
		temporal.Observer(nmtypes.AlphaPrice),
		logic.If(
			nomagique.Relay(calculus.SymbolReady, logic.SymbolCondition),
			nomagique.Pipe(
				calculus.LogRatio,
				nomagique.Relay(calculus.SymbolResult, SymbolChange),
			),
			nomagique.Identity,
		),
		temporal.Restart,
		calculus.Clear(calculus.SymbolTotal),
		nomagique.Assign(SymbolClosed, 1),
		statistic.Maturity(temporal.SymbolCompletedSpans),
	)
}

func openAcceleration() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.Assign(SymbolClosed, 0),
		statistic.Maturity(temporal.SymbolCompletedSpans),
	)
}
