package equation

import (
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
AdaptiveBaseline composes retention, central tendency, clustering, and capacity
control from universal primitives.
*/
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
current observation. Stability and capacity control still observe the updated
ring, so adaptation remains closed-loop without letting an observation dilute
the baseline used to score itself.
*/
func CausalBaseline() nomagique.Primitive {
	return nomagique.Pipe(
		nomagique.Retained(statistic.Mean),
		temporal.Window,
		statistic.Stability,
		temporal.Governor,
		nomagique.Relay(statistic.SymbolMean, statistic.SymbolBaseline),
	)
}
