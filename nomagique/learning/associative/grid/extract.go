package grid

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Extract validates the input feature vector channels for grid projection.
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
		var channels []float64

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			channels = append(channels, *(*float64)(arriving))
		}

		if len(channels) != op.channels {
			op.Error(core.ErrShape)
			return
		}

		for value := range data.NewValue(channels...).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
