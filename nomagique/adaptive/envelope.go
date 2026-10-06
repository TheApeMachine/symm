package adaptive

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Envelope replaces a value with the inclusive moment interval when the
estimator has dispersion.
*/
type Envelope struct {
	*core.PrimitiveError
	moments          core.Primitive
	coefficient      core.Primitive
	input            data.Map[string]
	coefficientInput data.Map[string]
	count            data.Map[float64]
	output           data.Map[float64]
}

func NewEnvelope(moments core.Primitive, coefficient core.Primitive) core.Primitive {
	count := data.NewOutputMap()
	count.Values["count"] = 0
	output := data.NewOutputMap()
	output.Values["value"] = 0

	return &Envelope{
		PrimitiveError:   core.NewPrimitiveError(),
		moments:          moments,
		coefficient:      coefficient,
		input:            data.NewMap("value", "value"),
		coefficientInput: data.NewMap("scale", "scale"),
		count:            count,
		output:           output,
	}
}

func (op *Envelope) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil || op.moments == nil || op.coefficient == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			value, ok := values.Values["value"]

			if !ok {
				op.Error(core.ErrNotHeld)
				return
			}

			var current [10]float64

			for pointer := range op.moments.Next(data.NewValue(value)) {
				current = *(*[10]float64)(pointer)
			}

			if err := op.moments.Error(); err != nil {
				op.Error(err)
				return
			}

			bounded := current[6]

			if current[0] > 1 && current[9] > 0 {
				op.count.Values["count"] = current[0]

				for range adapter.Next(data.NewValue(op.count)) {
				}

				for range op.coefficient.Next(data.NewValue(adapter)) {
				}

				if err := op.coefficient.Error(); err != nil {
					op.Error(err)
					return
				}

				var coefficient data.Map[float64]

				for pointer := range adapter.Next(data.NewValue(op.coefficientInput)) {
					coefficient = *(*data.Map[float64])(pointer)
				}

				if err := adapter.Error(); err != nil {
					op.Error(err)
					return
				}

				scale, held := coefficient.Values["scale"]

				if !held {
					op.Error(core.ErrNotHeld)
					return
				}

				margin := current[9] * scale
				lower := current[1] - margin
				upper := current[1] + margin

				if bounded < lower {
					bounded = lower
				}

				if bounded > upper {
					bounded = upper
				}
			}

			op.output.Values["value"] = bounded

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
