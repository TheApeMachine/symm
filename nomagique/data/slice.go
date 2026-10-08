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

func NewSlice(start, end int) core.Primitive {
	return &Slice{
		PrimitiveError: core.NewPrimitiveError(),
		Start:          start,
		End:            end,
	}
}

func (op *Slice) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		idx := 0
		for arriving := range in {
			if idx >= op.End {
				return
			}
			if idx >= op.Start {
				if !yield(arriving) {
					return
				}
			}
			idx++
		}
	}
}
