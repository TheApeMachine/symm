package learning

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
Forecast owns residual moments and the trust/scale recurrences. Mix supplies
both interpolations. The current residual participates in its surprise
statistic.

Each arrival is *[2]float64{predicted, actual}; it yields *[6]float64
{value, scale, trust, rate, count, weightCount}.
*/
type Forecast struct {
	*core.PrimitiveError
	moments core.Primitive
	trust   float64
	scale   float64
	rate    float64
	out     [6]float64
}

func NewForecast() core.Primitive {
	return &Forecast{
		PrimitiveError: core.NewPrimitiveError(),
		moments:        statistic.NewEstimator(),
		trust:          1,
		scale:          1,
	}
}

func (op *Forecast) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			pair := (*[2]float64)(arriving)
			predicted, actual := pair[0], pair[1]

			if math.IsNaN(predicted) || math.IsNaN(actual) ||
				math.IsInf(predicted, 0) || math.IsInf(actual, 0) {
				op.Error(core.ErrDomain)
				return
			}

			residual := actual - predicted
			var moments [10]float64

			for out := range op.moments.Next(data.NewValue(residual).Next(nil)) {
				moments = *(*[10]float64)(out)
			}

			if err := op.moments.Error(); err != nil {
				op.Error(err)
				return
			}

			if moments[0] > 1 {
				op.rate = 0

				if moments[8] > 0 {
					op.rate = math.Abs(residual-moments[1]) / math.Sqrt(moments[8])
				}

				target := math.Exp(math.Log(math.Abs(actual)) - math.Log(math.Abs(predicted)))

				for index, mix := range [2][3]float64{
					{op.trust, math.Max(0, 1-op.rate), op.rate},
					{op.scale, target, 0},
				} {
					if index == 1 {
						mix[2] = op.rate * (1 - op.trust)
					}

					mixed := (1-mix[2])*mix[0] + mix[2]*mix[1]

					if index == 0 {
						op.trust = mixed
						continue
					}

					op.scale = mixed
				}
			}

			op.out = [6]float64{op.scale, op.scale, op.trust, op.rate, moments[0], moments[0]}

			for value := range data.NewValue(op.out).Next(nil) {
				if !yield(value) {
					return
				}
			}
		}
	}
}
