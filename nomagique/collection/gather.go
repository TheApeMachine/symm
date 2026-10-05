package collection

import (
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Gather selects members at the configured indices.
*/
type Gather[T any] struct {
	*core.PrimitiveError
	indices []int
	out     []T
}

func NewGather[T any](indices []int) *Gather[T] {
	return &Gather[T]{
		PrimitiveError: core.NewPrimitiveError(),
		indices:        append([]int(nil), indices...),
	}
}

func (op *Gather[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			values := *(*[]T)(arriving)
			gathered := make([]T, 0, len(op.indices))
			ok := true

			for _, index := range op.indices {
				if index < 0 || index >= len(values) {
					op.Error(fmt.Errorf("%w: index %d of %d", core.ErrShape, index, len(values)))
					ok = false
					break
				}

				gathered = append(gathered, values[index])
			}

			if !ok {
				return
			}

			op.out = gathered

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
