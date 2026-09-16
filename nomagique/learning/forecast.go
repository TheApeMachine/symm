package learning

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
Pair is a predicted-versus-actual observation.
*/
type Pair struct {
	Predicted float64
	Actual    float64
}

/*
ForecastReading is the learned multiplicative scale and its trust.
*/
type ForecastReading struct {
	Value       float64
	Scale       float64
	Trust       float64
	Rate        float64
	Count       float64
	WeightCount float64
}

/*
Forecast owns residual moments and the trust/scale recurrences. Mix supplies
both interpolations. The current residual participates in its surprise
statistic.
*/
type Forecast struct {
	*core.PrimitiveError

	moments statistic.Moments
	mix     core.Primitive
	trust   float64
	scale   float64
	rate    float64
	out     ForecastReading
}

func NewForecast() *Forecast {
	return &Forecast{PrimitiveError: core.NewPrimitiveError(), mix: calculus.NewMix(),
		trust: 1,
		scale: 1,
	}
}

func (forecast *Forecast) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			pair := (*Pair)(arriving)

			if math.IsNaN(pair.Predicted) || math.IsNaN(pair.Actual) ||
				math.IsInf(pair.Predicted, 0) || math.IsInf(pair.Actual, 0) {
				forecast.Error(core.ErrDomain)
				return
			}

			residual := pair.Actual - pair.Predicted
			moments := forecast.moments.Update(residual)

			if moments.Count > 1 {
				forecast.rate = 0

				if moments.Variance > 0 {
					deviation := math.Abs(residual - moments.Mean)
					spread := math.Sqrt(moments.Variance)
					forecast.rate = deviation / spread
				}

				mixRec := calculus.MixRecord{
					Left:   forecast.trust,
					Right:  math.Max(0, 1-forecast.rate),
					Weight: forecast.rate,
				}

				var trust float64

				for out := range forecast.mix.Next(sequence.NewValues(mixRec).Next(nil)) {
					trust = *(*float64)(out)
				}

				if err := forecast.mix.Error(); err != nil {
					forecast.Error(err)
					return
				}

				forecast.trust = trust

				actualAbs := math.Abs(pair.Actual)
				predictedAbs := math.Abs(pair.Predicted)

				target := math.Exp(math.Log(actualAbs) - math.Log(predictedAbs))

				scaleMix := calculus.MixRecord{
					Left:   forecast.scale,
					Right:  target,
					Weight: forecast.rate * (1 - forecast.trust),
				}

				var scale float64

				for out := range forecast.mix.Next(sequence.NewValues(scaleMix).Next(nil)) {
					scale = *(*float64)(out)
				}

				if err := forecast.mix.Error(); err != nil {
					forecast.Error(err)
					return
				}

				forecast.scale = scale
			}

			forecast.out = ForecastReading{
				Value:       forecast.scale,
				Scale:       forecast.scale,
				Trust:       forecast.trust,
				Rate:        forecast.rate,
				Count:       moments.Count,
				WeightCount: moments.Count,
			}

			if !yield(unsafe.Pointer(&forecast.out)) {
				return
			}
		}
	}
}
