package geometry

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Border identifies saddle-point boundary edges where distinct basins meet.
*/
type Border struct {
	*core.PrimitiveError
}

func NewBorder() *Border {
	return &Border{PrimitiveError: core.NewPrimitiveError()}
}

func (border *Border) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			if border.Error() != nil {
				return
			}

			if arriving == nil {
				continue
			}

			edge := (*Edge)(arriving)
			edge.Border = edge.Basin[0] != edge.Basin[1]

			if !yield(unsafe.Pointer(edge)) {
				return
			}
		}
	}
}
