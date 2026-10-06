package adaptive

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Threshold composes a moment estimator with a dispersion coefficient.
A source with no dispersion has threshold one.
*/
type Threshold struct {
	*core.PrimitiveError
	moments          core.Primitive
	coefficient      core.Primitive
	input            data.Map[string]
	coefficientInput data.Map[string]
	count            data.Map[float64]
	output           data.Map[float64]
}

func NewThreshold(moments core.Primitive, coefficient core.Primitive) core.Primitive {
	count := data.NewOutputMap()
	count.Values["count"] = 0
	output := data.NewOutputMap()
	output.Values["threshold"] = 1

	return &Threshold{
		PrimitiveError:   core.NewPrimitiveError(),
		moments:          moments,
		coefficient:      coefficient,
		input:            data.NewMap("value", "value"),
		coefficientInput: data.NewMap("scale", "scale"),
		count:            count,
		output:           output,
	}
}

func (op *Threshold) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			threshold := 1.0

			if current[9] > 0 {
				threshold = current[9] * scale
			}

			op.output.Values["threshold"] = threshold

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
