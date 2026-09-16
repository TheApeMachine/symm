package sequence

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
	return &Gather[T]{PrimitiveError: core.NewPrimitiveError(), indices: append([]int(nil), indices...)}
}

func (gather *Gather[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			values := *(*[]T)(arriving)
			gathered := make([]T, 0, len(gather.indices))
			ok := true

			for _, index := range gather.indices {
				if index < 0 || index >= len(values) {
					gather.Error(fmt.Errorf("%w: index %d of %d", core.ErrShape, index, len(values)))
					ok = false
					break
				}

				gathered = append(gathered, values[index])
			}

			if !ok {
				return
			}

			gather.out = gathered

			if !yield(unsafe.Pointer(&gather.out)) {
				return
			}
		}
	}
}
