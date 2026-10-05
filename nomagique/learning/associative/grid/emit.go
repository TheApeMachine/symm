package grid

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Emit gathers region condition tokens and yields them as a completed step slice.
Input: *uint64.
Yields: *[]uint64 (length = regions).
*/
type Emit struct {
	*core.PrimitiveError
	regions int
}

func NewEmit(regions int) core.Primitive {
	return &Emit{
		PrimitiveError: core.NewPrimitiveError(),
		regions:        regions,
	}
}

func (op *Emit) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		batch := make([]uint64, 0, op.regions)

		for arriving := range in {
			if arriving == nil {
				continue
			}

			token := *(*uint64)(arriving)
			batch = append(batch, token)

			if len(batch) == op.regions {
				out := make([]uint64, op.regions)
				copy(out, batch)
				batch = batch[:0]

				if !yield(unsafe.Pointer(&out)) {
					return
				}
			}
		}
	}
}
