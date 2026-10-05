package grid

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Extract validates and extracts the input feature vector for grid projection.
Input: *[]float64 (length = channels).
Yields: *[]float64 (length = channels).
*/
type Extract struct {
	*core.PrimitiveError
	channels int
}

func NewExtract(channels int) *Extract {
	return &Extract{
		PrimitiveError: core.NewPrimitiveError(),
		channels:       channels,
	}
}

func (op *Extract) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				continue
			}

			vec := *(*[]float64)(arriving)

			if len(vec) != op.channels {
				op.Error(core.ErrShape)
				return
			}

			if !yield(unsafe.Pointer(&vec)) {
				return
			}
		}
	}
}
