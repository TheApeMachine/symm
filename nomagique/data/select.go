package data

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Select extracts specific items from the upstream sequence by index and yields them.
It consumes the entire upstream sequence into memory before yielding.
*/
type Select struct {
	*core.PrimitiveError
	Indices []int
}

func NewSelect(indices ...int) core.Primitive {
	return &Select{
		PrimitiveError: core.NewPrimitiveError(),
		Indices:        indices,
	}
}

func (op *Select) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var items []unsafe.Pointer
		for arriving := range in {
			items = append(items, arriving)
		}

		for _, idx := range op.Indices {
			if idx >= 0 && idx < len(items) {
				if !yield(items[idx]) {
					return
				}
			}
		}
	}
}
