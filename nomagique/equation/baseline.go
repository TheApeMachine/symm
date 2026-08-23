package equation

import (
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
)

// AdaptiveBaseline composes retention, center, clustering, and capacity control.
func AdaptiveBaseline() nomagique.Primitive {
	return nomagique.Pipe(
		temporal.Window,
		statistic.Mean,
		statistic.Stability,
		temporal.Governor,
	)
}

/*
CausalBaseline exposes the center of committed samples before retaining the
current observation, then explicitly publishes that center as the baseline fact.
*/
func CausalBaseline() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.Retained(statistic.Mean),
		temporal.Window,
		statistic.Stability,
		temporal.Governor,
		nomagique.Wire(
			nomagique.Identity,
			nomagique.In(statistic.SymbolMean, calculus.PortX),
			nomagique.Out(calculus.PortX, statistic.SymbolBaseline),
		),
	)
}
