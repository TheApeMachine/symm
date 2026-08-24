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
		temporal.Window(""),
		statistic.Median,
		nmtypes.Wire(
			nmtypes.Identity,
			nmtypes.In(statistic.SymbolResult, SymbolTarget),
			nmtypes.Out(SymbolTarget, SymbolTarget),
		),
		nmtypes.Wire(
			nmtypes.Identity,
			nmtypes.In(SymbolTarget, calculus.SymbolBaseline),
			nmtypes.Out(calculus.SymbolBaseline, calculus.SymbolBaseline),
		),
		temporal.Since,
		nmtypes.Wire(
			calculus.Accumulate,
			nmtypes.In(nmtypes.Quantity, calculus.SymbolDelta),
			nmtypes.State(calculus.SymbolTotal, calculus.SymbolTotal),
			nmtypes.Out(calculus.SymbolTotal, calculus.SymbolTotal),
		),
		logic.If(accelerationClosed(), closeAcceleration(), openAcceleration()),
	)
}

func accelerationClosed() nmtypes.Primitive {
	return nmtypes.Pipe(
		nmtypes.Fork(
			nmtypes.Wire(
				logic.GreaterOrEqual,
				nmtypes.In(calculus.SymbolTotal, calculus.PortA),
				nmtypes.In(calculus.SymbolBaseline, calculus.PortB),
				nmtypes.Out(logic.SymbolCondition, calculus.PortA),
			),
			nmtypes.Wire(
				nmtypes.Identity,
				nmtypes.In(temporal.SymbolAdvanced, calculus.PortB),
				nmtypes.Out(calculus.PortB, calculus.PortB),
			),
		),
		logic.And,
	)
}

func closeAcceleration() nmtypes.Primitive {
	return nmtypes.Pipe(
		nmtypes.Wire(
			calculus.Rate,
			nmtypes.In(calculus.SymbolTotal, calculus.SymbolCount),
			nmtypes.In(calculus.SymbolDuration, calculus.SymbolDuration),
			nmtypes.Out(calculus.SymbolRate, calculus.SymbolRate),
		),
		temporal.Observer("", nmtypes.AlphaPrice),
		logic.If(
			nmtypes.Wire(
				nmtypes.Identity,
				nmtypes.In(calculus.SymbolReady, logic.SymbolCondition),
				nmtypes.Out(logic.SymbolCondition, logic.SymbolCondition),
			),
			nmtypes.Wire(
				calculus.LogRatio,
				nmtypes.In(calculus.SymbolCurrent, calculus.SymbolCurrent),
				nmtypes.In(calculus.SymbolPrevious, calculus.SymbolPrevious),
				nmtypes.Out(calculus.PortResult, SymbolChange),
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
