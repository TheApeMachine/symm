package temporal

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Timestamp converts or retains a timestamp coordinate.
*/
type Timestamp struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewTimestamp() *Timestamp {
	output := data.NewOutputMap()
	output.Values["timestamp"] = 0

	return &Timestamp{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("at", "at"),
		output:         output,
	}
}

func (op *Timestamp) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			at, ok := values.Values["at"]

			if !ok {
				op.Error(core.ErrNotHeld)
				return
			}

			op.output.Values["timestamp"] = at

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
