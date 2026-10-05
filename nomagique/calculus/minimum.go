package calculus

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Minimum tracks the running minimum after every arrival.
*/
type Minimum struct {
	*core.PrimitiveError
	acc    float64
	seen   bool
	input  data.Map[string]
	output data.Map[float64]
}

func NewMinimum(current ...float64) *Minimum {
	output := data.NewOutputMap()
	output.Values["value"] = 0
	output.Values["min"] = 0

	op := &Minimum{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("value", "value"),
		output:         output,
	}

	if len(current) > 0 {
		op.acc = current[0]
		op.seen = true
		op.output.Values["value"] = current[0]
		op.output.Values["min"] = current[0]
	}

	return op
}

func (op *Minimum) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			if !op.seen {
				op.seen = true
				op.acc = val
			}

			if val < op.acc {
				op.acc = val
			}

			op.output.Values["value"] = op.acc
			op.output.Values["min"] = op.acc

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
