package calculus

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/* Log owns one natural-log field operation. */
type Log struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewLog() core.Primitive {
	output := data.NewOutputMap()
	output.Values["log"] = 0

	return &Log{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("value", "value"),
		output:         output,
	}
}

func (op *Log) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			adapter := *(**data.Adapter)(arriving)
			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			value, ok := values.Values["value"]

			if !ok {
				if !yield(arriving) {
					return
				}
				continue
			}

			if value <= 0 {
				op.Error(core.ErrDomain)
				return
			}

			op.output.Values["log"] = math.Log(value)

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
