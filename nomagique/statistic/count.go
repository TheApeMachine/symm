package statistic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Count counts delivered objects, regardless of their payload.
*/
type Count struct {
	*core.PrimitiveError
	out float64
}

func NewCount() *Count {
	return &Count{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Count) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			op.out++

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
