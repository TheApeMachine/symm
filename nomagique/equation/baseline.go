package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

// AdaptiveBaseline composes retention, center, clustering, and capacity control.
func AdaptiveBaseline() types.Primitive {
	return types.Pipe(
		temporal.Window(""),
		statistic.Mean,
		statistic.Stability(""),
		temporal.Governor,
	)
}

/*
CausalBaseline exposes the center of committed samples before retaining the
current observation, then explicitly publishes that center as the baseline fact.
*/
func CausalBaseline() types.Primitive {
	return types.Pipe(
		statistic.Mean,
		temporal.Window(""),
		statistic.Stability(""),
		temporal.Governor,
		types.Wire(
			types.Identity,
			types.In(statistic.SymbolMean, calculus.PortX),
			types.Out(calculus.PortX, statistic.SymbolBaseline),
		),
	)
}
