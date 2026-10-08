package data

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

type Slice struct {
	*core.PrimitiveError
	Start int
	End   int
}

// NewSlice selects [start,end). Omitting end retains the remainder of the run.
func NewSlice(start int, end ...int) core.Primitive {
	op := &Slice{PrimitiveError: core.NewPrimitiveError(), Start: start, End: -1}

	if start < 0 || len(end) > 1 {
		op.Error(core.ErrShape)
		return op
	}

	if len(end) == 1 {
		op.End = end[0]

		if op.End < start {
			op.Error(core.ErrShape)
		}
	}

	return op
}

func (op *Slice) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if op.Error() != nil {
			return
		}

		index := 0

		for arriving := range in {
			if op.End >= 0 && index >= op.End {
				return
			}

			if index >= op.Start && !yield(arriving) {
				return
			}

			index++
		}
	}
}
