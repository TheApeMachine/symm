package collection

import (
	"fmt"
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Gather selects members at the configured indices. At owns single-index
selection; Gather owns a list of them.
*/
type Gather[T any] struct {
	core.Base[[]T, []T]
	indices []int
}

func NewGather[T any](indices []int) *Gather[T] {
	return &Gather[T]{indices: append([]int(nil), indices...)}
}

func (op *Gather[T]) Next(
	in iter.Seq[core.Primitive[[]T, []T]],
) iter.Seq[core.Primitive[[]T, []T]] {
	return func(yield func(core.Primitive[[]T, []T]) bool) {
		for arriving := range in {
			values := arriving.Read()
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
				continue
			}

			if !yield(op.Carrier(gathered)) {
				return
			}
		}
	}
}
