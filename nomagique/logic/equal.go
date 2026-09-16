package logic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Equal owns one equality relation. Pairing is external: what arrives is already
two values.
*/
type Equal struct {
	*core.PrimitiveError

	out bool
}

func NewEqual() *Equal {
	return &Equal{PrimitiveError: core.NewPrimitiveError()}
}

func (equal *Equal) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*[2]float64)(arriving)
			equal.out = in[0] == in[1]

			if !yield(unsafe.Pointer(&equal.out)) {
				return
			}
		}
	}
}
