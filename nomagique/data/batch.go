package data

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Batch partitions an inbound sequence of arrivals into discrete groups of sizes
specified by `sizes`. It yields each group as a data.Value primitive.
*/
type Batch struct {
	*core.PrimitiveError
	sizes []int
}

func NewBatch(sizes ...int) core.Primitive {
	return &Batch{
		PrimitiveError: core.NewPrimitiveError(),
		sizes:          sizes,
	}
}

func (op *Batch) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		groups := make([][]unsafe.Pointer, len(op.sizes))

		for index, size := range op.sizes {
			groups[index] = make([]unsafe.Pointer, 0, size)
		}

		groupIdx := 0

		for arriving := range in {
			if arriving == nil {
				continue
			}

			if groupIdx >= len(op.sizes) {
				break
			}

			groups[groupIdx] = append(groups[groupIdx], arriving)

			if len(groups[groupIdx]) == op.sizes[groupIdx] {
				groupIdx++
			}
		}

		if groupIdx < len(op.sizes) {
			op.Error(core.ErrShape)
			return
		}

		for _, groupItems := range groups {
			var group core.Primitive = &Value[unsafe.Pointer]{
				PrimitiveError: core.NewPrimitiveError(),
				Values:         groupItems,
			}

			if !yield(unsafe.Pointer(&group)) {
				return
			}
		}
	}
}
