package logic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Not inverts each arrival.
*/
type Not struct {
	*core.PrimitiveError

	out bool
}

func NewNot() *Not {
	return &Not{PrimitiveError: core.NewPrimitiveError()}
}

func (not *Not) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*bool)(arriving)
			not.out = !*in

			if !yield(unsafe.Pointer(&not.out)) {
				return
			}
		}
	}
}
