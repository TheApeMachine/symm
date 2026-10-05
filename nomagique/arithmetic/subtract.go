package arithmetic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Subtract owns one field operation in its native coordinates:
minuend - subtrahend = difference.
*/
type Subtract struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewSubtract() core.Primitive {
	output := data.NewOutputMap()
	output.Values["difference"] = 0

	return &Subtract{
		PrimitiveError: core.NewPrimitiveError(),
		input: data.NewMap(
			"minuend", "minuend",
			"subtrahend", "subtrahend",
		),
		output: output,
	}
}

func (op *Subtract) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			minuend, minuendOK := values.Values["minuend"]
			subtrahend, subtrahendOK := values.Values["subtrahend"]

			if !minuendOK || !subtrahendOK {
				if !yield(arriving) {
					return
				}

				continue
			}

			op.output.Values["difference"] = minuend - subtrahend

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
