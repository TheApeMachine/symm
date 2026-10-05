package calculus

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Reciprocal owns multiplicative inverse transformation of an arrival.
*/
type Reciprocal struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewReciprocal() *Reciprocal {
	output := data.NewOutputMap()
	output.Values["value"] = 0
	output.Values["reciprocal"] = 0

	return &Reciprocal{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("value", "value"),
		output:         output,
	}
}

func (op *Reciprocal) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			if val == 0 {
				op.Error(core.ErrDomain)
				return
			}

			result := 1.0 / val
			op.output.Values["value"] = result
			op.output.Values["reciprocal"] = result

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
