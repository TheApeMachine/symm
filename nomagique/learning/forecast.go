package learning

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/transport"
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
	err     error
	moments statistic.Moments
	mix     core.Primitive
	trust   float64
	scale   float64
	rate    float64
	out     ForecastReading
}

func NewForecast() core.Primitive {
	return &Forecast{
		mix:   calculus.NewMix(),
		trust: 1,
		scale: 1,
	}
}

func (op *Forecast) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			pair := (*Pair)(arriving)

			if math.IsNaN(pair.Predicted) || math.IsNaN(pair.Actual) ||
				math.IsInf(pair.Predicted, 0) || math.IsInf(pair.Actual, 0) {
				op.Error(core.ErrDomain)
				return
			}

			residual := pair.Actual - pair.Predicted
			moments := op.moments.Update(residual)

			if moments.Count > 1 {
				op.rate = 0

				if moments.Variance > 0 {
					deviation := math.Abs(residual - moments.Mean)
					spread := math.Sqrt(moments.Variance)
					op.rate = deviation / spread
				}

				mixRec := calculus.MixRecord{
					Left:   op.trust,
					Right:  math.Max(0, 1-op.rate),
					Weight: op.rate,
				}

				var trust float64

				for out := range op.mix.Next(transport.NewValues(mixRec).Next(nil)) {
					trust = *(*float64)(out)
				}

				if err := op.mix.Error(); err != nil {
					op.Error(err)
					return
				}

				op.trust = trust

				actualAbs := math.Abs(pair.Actual)
				predictedAbs := math.Abs(pair.Predicted)

				target := math.Exp(math.Log(actualAbs) - math.Log(predictedAbs))

				scaleMix := calculus.MixRecord{
					Left:   op.scale,
					Right:  target,
					Weight: op.rate * (1 - op.trust),
				}

				var scale float64

				for out := range op.mix.Next(transport.NewValues(scaleMix).Next(nil)) {
					scale = *(*float64)(out)
				}

				if err := op.mix.Error(); err != nil {
					op.Error(err)
					return
				}

				op.scale = scale
			}

			op.out = ForecastReading{
				Value:       op.scale,
				Scale:       op.scale,
				Trust:       op.trust,
				Rate:        op.rate,
				Count:       moments.Count,
				WeightCount: moments.Count,
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Forecast) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
