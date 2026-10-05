package logic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Less owns one ordering relation. Pairing is external: what arrives is already
two values.
*/
type Less struct {
	*core.PrimitiveError
	out bool
}

func NewLess() *Less {
	return &Less{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Less) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			pair := (*[2]float64)(arriving)
			op.out = pair[0] < pair[1]

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
