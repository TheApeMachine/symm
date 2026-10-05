package calculus

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Log owns the natural logarithm in its native coordinates:
argument -> logarithm.
*/
type Log struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewLog() core.Primitive {
	output := data.NewOutputMap()
	output.Values["logarithm"] = 0

	return &Log{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("argument", "argument"),
		output:         output,
	}
}

func (op *Log) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			if !ok || argument <= 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			op.output.Values["logarithm"] = math.Log(argument)

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
