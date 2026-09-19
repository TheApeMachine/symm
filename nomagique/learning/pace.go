package learning

import (
	"math"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Pace is the adaptive learning rate controller, built entirely via functional composition
of canonical mathematical and statistical atoms.

It takes an error magnitude and yields a bounded exponential moving average of an
adapted learning rate, scaled by the empirical rank of the incoming error.
*/
func Pace(rest, lower, upper, gain, band types.Float, window types.Integer) types.Value[float64, float64] {
	restVal := rest(nil)
	lowerVal := lower(nil)
	upperVal := upper(nil)
	gainVal := gain(nil)
	bandVal := band(nil)
	windowVal := window(nil)

	return types.Value[float64, float64](nomagique.NewNumber(
		// 1. Maintain history and map error magnitude to empirical rank
		types.Value[float64, float64](probability.NewCalibrator(
			types.Value[[]float64, []float64](sequence.NewTail[float64](types.Const(windowVal))),
		)),

		// 2. Map the rank to a target log-alpha based on the bands
		types.Value[float64, float64](statistic.NewThreshold(
			types.Const(bandVal),
			types.Const(math.Log(restVal)),
			types.Const(math.Log(lowerVal)),
			types.Const(math.Log(upperVal)),
		)),

		// 3. Smooth the target log-alpha with an Exponential Moving Average
		types.Value[float64, float64](statistic.NewEMA(types.Const(gainVal))),

		// 4. Clamp the internal log-alpha to bounds
		types.Value[float64, float64](arithmetic.NewClamp(types.Const(math.Log(lowerVal)), types.Const(math.Log(upperVal)))),

		// 5. Convert back from log-space to time-space
		types.Value[float64, float64](arithmetic.NewExp()),

		// 6. Clamp the final alpha to hard bounds
		types.Value[float64, float64](arithmetic.NewClamp(types.Const(lowerVal), types.Const(upperVal))),
	))
}
