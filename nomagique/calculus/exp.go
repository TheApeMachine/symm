package calculus

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Exp owns the exponential in its native coordinates:
exponent -> exponential.
*/
type Exp struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewExp() core.Primitive {
	output := data.NewOutputMap()
	output.Values["exponential"] = 0

	return &Exp{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("exponent", "exponent"),
		output:         output,
	}
}

func (op *Exp) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			exponent, ok := values.Values["exponent"]

			if !ok {
				if !yield(arriving) {
					return
				}

				continue
			}

			op.output.Values["exponential"] = math.Exp(exponent)

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
