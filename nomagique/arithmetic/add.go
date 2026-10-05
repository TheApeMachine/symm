package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Add owns one field operation in its native coordinates:
augend + addend = sum.
*/
type Add struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewAdd() core.Primitive {
	output := data.NewOutputMap()
	output.Values["sum"] = 0

	return &Add{
		PrimitiveError: core.NewPrimitiveError(),
		input: data.NewMap(
			"augend", "augend",
			"addend", "addend",
		),
		output: output,
	}
}

func (op *Add) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			augend, augendOK := values.Values["augend"]
			addend, addendOK := values.Values["addend"]

			if !augendOK || !addendOK {
				if !yield(arriving) {
					return
				}

				continue
			}

			op.output.Values["sum"] = augend + addend

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
