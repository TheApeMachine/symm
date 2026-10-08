package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

// Parallel routes input group i to branch i and yields results in branch order.
// A group is a core.Primitive whose Next yields its operands individually.
// Evaluation is ordered: the former goroutines only built lazy iterators and
// did not execute the branches in parallel.
type Parallel struct {
	*core.PrimitiveError
	branches []core.Primitive
}

func NewParallel(branches ...core.Primitive) core.Primitive {
	return &Parallel{PrimitiveError: core.NewPrimitiveError(), branches: branches}
}

func (op *Parallel) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if op.Error() != nil {
			return
		}

		values := make([]core.Primitive, 0, len(op.branches))

		for arriving := range in {
			if arriving == nil || len(values) == len(op.branches) {
				op.Error(core.ErrShape)
				return
			}

			value := *(*core.Primitive)(arriving)

			if value == nil {
				op.Error(core.ErrShape)
				return
			}

			values = append(values, value)
		}

		if len(values) != len(op.branches) {
			op.Error(core.ErrShape)
			return
		}

		for _, branch := range op.branches {
			if branch == nil {
				op.Error(core.ErrShape)
				return
			}
		}

		for index, branch := range op.branches {
			if err := op.Error(branch.Error(), values[index].Error()); err != nil {
				return
			}

			for value := range branch.Next(values[index].Next(nil)) {
				if value == nil {
					op.Error(core.ErrShape)
					return
				}

				if !yield(value) {
					return
				}
			}

			if err := op.Error(branch.Error(), values[index].Error()); err != nil {
				return
			}
		}
	}
}
