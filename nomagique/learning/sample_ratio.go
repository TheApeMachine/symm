package learning

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
SampleRatio preserves the supplied calibration ratio and observed-range
ceiling over the shared residual-span update. Those are model policies, not
statistical identities.

Each arrival is *[2]float64{predicted, actual}; it yields *[3]float64
{value, peakRatio, count}.
*/
type SampleRatio struct {
	*core.PrimitiveError
	span      core.Primitive
	abs       core.Primitive
	adapter   *data.Adapter
	publish   data.Map[float64]
	request   data.Map[string]
	count     float64
	min       float64
	max       float64
	prev      float64
	peakRatio float64
	out       [3]float64
}

func NewSampleRatio() core.Primitive {
	return &SampleRatio{
		PrimitiveError: core.NewPrimitiveError(),
		span:           statistic.NewResidualSpan(),
		abs:            calculus.NewAbsolute(),
		adapter:        data.NewAdapter(nil, data.NewState(data.NewMap())),
		publish:        data.NewOutputMap(),
		request:        data.NewMap("absolute", "absolute"),
	}
}

func (op *SampleRatio) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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
			var span [4]float64

			for out := range op.span.Next(data.NewValue([4]float64{op.count, op.min, op.max, residual})) {
				span = *(*[4]float64)(out)
			}

			if err := op.span.Error(); err != nil {
				op.Error(err)
				return
			}

			op.count = span[0]
			op.min = span[1]
			op.max = span[2]
			ratio := actual / predicted

			if actual < predicted {
				ratio = 1 + actual/predicted
			}

			if !(predicted <= actual || ratio >= 0) {
				op.Error(core.ErrDomain)
				return
			}

			ceiling := 1.0

			if span[3] > 0 {
				ceiling = 1 + 1/span[3]
			}

			if !(span[3] > 0) {
				op.publish.Values["value"] = op.prev

				for range op.adapter.Next(data.NewValue(op.publish)) {
				}

				for range op.abs.Next(data.NewValue(op.adapter)) {
				}

				if err := op.abs.Error(); err != nil {
					op.Error(err)
					return
				}

				prevAbs := 0.0

				for pointer := range op.adapter.Next(data.NewValue(op.request)) {
					prevAbs = (*data.Map[float64])(pointer).Values["absolute"]
				}

				if err := op.adapter.Error(); err != nil {
					op.Error(err)
					return
				}

				ceiling = 1 + 1/prevAbs
			}

			if ratio > ceiling {
				ratio = ceiling
			}

			if ratio > op.peakRatio {
				op.peakRatio = ratio
			}

			op.prev = predicted
			op.out = [3]float64{ratio, op.peakRatio, op.count}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
