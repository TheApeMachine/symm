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
