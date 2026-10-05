package calculus

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Atanh owns inverse hyperbolic tangent transformation of an arrival.
*/
type Atanh struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewAtanh() *Atanh {
	output := data.NewOutputMap()
	output.Values["value"] = 0
	output.Values["atanh"] = 0

	return &Atanh{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("value", "value"),
		output:         output,
	}
}

func (op *Atanh) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
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

			val, ok := values.Values["value"]

			if !ok {
				op.Error(core.ErrNotHeld)
				return
			}

			if val <= -1.0 || val >= 1.0 {
				op.Error(core.ErrDomain)
				return
			}

			result := math.Atanh(val)
			op.output.Values["value"] = result
			op.output.Values["atanh"] = result

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
