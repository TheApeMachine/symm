package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Multiply owns one field operation in its native coordinates:
multiplicand * multiplier = product.
*/
type Multiply struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewMultiply() core.Primitive {
	output := data.NewOutputMap()
	output.Values["product"] = 0

	return &Multiply{
		PrimitiveError: core.NewPrimitiveError(),
		input: data.NewMap(
			"multiplicand", "multiplicand",
			"multiplier", "multiplier",
		),
		output: output,
	}
}

func (op *Multiply) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			multiplicand, multiplicandOK := values.Values["multiplicand"]
			multiplier, multiplierOK := values.Values["multiplier"]

			if !multiplicandOK || !multiplierOK {
				if !yield(arriving) {
					return
				}

				continue
			}

			op.output.Values["product"] = multiplicand * multiplier

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
