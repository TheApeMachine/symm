package temporal

import (
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Elapsed subtracts int64 nanoseconds before conversion to seconds so epoch
magnitude cannot erase a small interval by cancellation.
*/
type Elapsed struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewElapsed() *Elapsed {
	output := data.NewOutputMap()
	output.Values["elapsed"] = 0

	return &Elapsed{
		PrimitiveError: core.NewPrimitiveError(),
		input: data.NewMap(
			"from", "from",
			"to", "to",
		),
		output: output,
	}
}

func (op *Elapsed) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			from, fromOK := values.Values["from"]
			to, toOK := values.Values["to"]

			if !fromOK || !toOK {
				op.Error(core.ErrNotHeld)
				return
			}

			op.output.Values["elapsed"] = (to - from) / float64(time.Second)

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
