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
TrustWeight owns the residual-range trust recurrence. No clipping is added to
the trust recurrence; rate greater than one retains the original extrapolating
behavior. Invalid inputs fail before entering retained state.

Each arrival is *[2]float64{predicted, actual}; it yields *[4]float64
{value, trust, rate, count}.
*/
type TrustWeight struct {
	*core.PrimitiveError
	span     core.Primitive
	mix      core.Primitive
	abs      core.Primitive
	adapter  *data.Adapter
	publish  data.Map[float64]
	absolute data.Map[string]
	mixed    data.Map[string]
	count    float64
	min      float64
	max      float64
	trust    float64
	rate     float64
	out      [4]float64
}

func NewTrustWeight() core.Primitive {
	return &TrustWeight{
		PrimitiveError: core.NewPrimitiveError(),
		span:           statistic.NewResidualSpan(),
		mix:            calculus.NewMix(),
		abs:            calculus.NewAbsolute(),
		adapter:        data.NewAdapter(nil, data.NewState(data.NewMap())),
		publish:        data.NewOutputMap(),
		absolute:       data.NewMap("absolute", "absolute"),
		mixed:          data.NewMap("mix", "mix"),
		trust:          1,
	}
}

func (op *TrustWeight) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			if op.count > 1 {
				if !(span[3] > 0) {
					op.Error(core.ErrDomain)
					return
				}

				op.publish.Values["value"] = residual

				for range op.adapter.Next(data.NewValue(op.publish)) {
				}

				for range op.abs.Next(data.NewValue(op.adapter)) {
				}

				if err := op.abs.Error(); err != nil {
					op.Error(err)
					return
				}

				magnitude := 0.0

				for pointer := range op.adapter.Next(data.NewValue(op.absolute)) {
					magnitude = (*data.Map[float64])(pointer).Values["absolute"]
				}

				op.rate = magnitude / span[3]
				op.publish.Values["left"] = op.trust
				op.publish.Values["right"] = math.Max(0, 1-op.rate)
				op.publish.Values["weight"] = op.rate

				for range op.adapter.Next(data.NewValue(op.publish)) {
				}

				for range op.mix.Next(data.NewValue(op.adapter)) {
				}

				if err := op.mix.Error(); err != nil {
					op.Error(err)
					return
				}

				for pointer := range op.adapter.Next(data.NewValue(op.mixed)) {
					op.trust = (*data.Map[float64])(pointer).Values["mix"]
				}

				if err := op.adapter.Error(); err != nil {
					op.Error(err)
					return
				}
			}

			op.out = [4]float64{op.trust, op.trust, op.rate, op.count}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
