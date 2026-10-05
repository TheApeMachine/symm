package statistic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/* Count counts delivered Adapter observations. */
type Count struct {
	*core.PrimitiveError
	count  float64
	output data.Map[float64]
}

func NewCount() core.Primitive {
	output := data.NewOutputMap()
	output.Values["count"] = 0

	return &Count{
		PrimitiveError: core.NewPrimitiveError(),
		output:         output,
	}
}

func (op *Count) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			op.count++
			op.output.Values["count"] = op.count

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
