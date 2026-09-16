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
	return &Count{PrimitiveError: core.NewPrimitiveError()}
}

func (count *Count) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for range in {
			count.out++

			if !yield(unsafe.Pointer(&count.out)) {
				return
			}
		}
	}
}
