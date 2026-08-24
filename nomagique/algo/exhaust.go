package algo

import (
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
Exhaust evaluates the four physical decay channels (mechanical depth collapse,
fragile spread expansion, thermal price rejection, and directional reversal)
and fuses them into an urgency margin.
*/
func Exhaust() nmtypes.Primitive {
	return nmtypes.Pipe(
		nmtypes.Wire(
			nmtypes.Identity,
			nmtypes.In(statistic.SymbolVolume, nmtypes.SampleValue),
			nmtypes.Out(nmtypes.SampleValue, nmtypes.SampleValue),
		),
		nmtypes.Configure(
			statistic.Baseline,
			nmtypes.Span,
			temporal.Window,
		),
		statistic.ExhaustScores,
	)
}
