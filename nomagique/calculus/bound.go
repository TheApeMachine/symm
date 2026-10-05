package calculus

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Bound clamps value between lower and upper bounds.
*/
type Bound struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewBound() *Bound {
	output := data.NewOutputMap()
	output.Values["value"] = 0
	output.Values["bound"] = 0

	return &Bound{
		PrimitiveError: core.NewPrimitiveError(),
		input: data.NewMap(
			"value", "value",
			"lower", "lower",
			"upper", "upper",
		),
		output: output,
	}
}

func (op *Bound) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			val, valOK := values.Values["value"]
			lower, lowerOK := values.Values["lower"]
			upper, upperOK := values.Values["upper"]

			if !valOK || !lowerOK || !upperOK {
				op.Error(core.ErrNotHeld)
				return
			}

			if lower > upper {
				op.Error(core.ErrDomain)
				return
			}

			result := val

			if result < lower {
				result = lower
			}

			if result > upper {
				result = upper
			}

			op.output.Values["value"] = result
			op.output.Values["bound"] = result

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
