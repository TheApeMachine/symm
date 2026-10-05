package calculus

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Sign owns the signum transformation in its native coordinates:
argument -> sign.
*/
type Sign struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewSign() core.Primitive {
	output := data.NewOutputMap()
	output.Values["sign"] = 0

	return &Sign{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("argument", "argument"),
		output:         output,
	}
}

func (op *Sign) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			argument, ok := values.Values["argument"]

			if !ok {
				if !yield(arriving) {
					return
				}

				continue
			}

			sign := 0.0

			if argument > 0 {
				sign = 1
			}

			if argument < 0 {
				sign = -1
			}

			op.output.Values["sign"] = sign

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
