package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

// Fanout sends the same frozen run to each branch. It composes Pack and
// Parallel; unlike Value, it evaluates the supplied computations.
type Fanout[T any] struct {
	*core.PrimitiveError
	pack     core.Primitive
	parallel core.Primitive
	count    int
}

func NewFanout[T any](branches ...core.Primitive) core.Primitive {
	return &Fanout[T]{
		PrimitiveError: core.NewPrimitiveError(),
		pack:           data.NewPack[T](),
		parallel:       NewParallel(branches...),
		count:          len(branches),
	}
}

func (op *Fanout[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for pointer := range op.pack.Next(in) {
			group := *(*core.Primitive)(pointer)
			groups := make([]core.Primitive, op.count)

			for index := range groups {
				groups[index] = group
			}

			for value := range op.parallel.Next(data.NewValue(groups...).Next(nil)) {
				if !yield(value) {
					return
				}
			}
		}

		op.Error(op.pack.Error(), op.parallel.Error())
	}
}
