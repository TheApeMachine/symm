package grid

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Emit gathers region condition tokens and validates them as a completed step.
*/
type Emit struct {
	*core.PrimitiveError
	regions int
}

func NewEmit(regions int) *Emit {
	return &Emit{
		PrimitiveError: core.NewPrimitiveError(),
		regions:        regions,
	}
}

func (op *Emit) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var tokens []float64

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			tokens = append(tokens, *(*float64)(arriving))
		}

		if len(tokens) != op.regions {
			op.Error(core.ErrShape)
			return
		}

		for value := range data.NewValue(tokens...).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
