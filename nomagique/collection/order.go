package collection

import (
	"cmp"
	"errors"
	"iter"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Order owns ordering a collection.
*/
type Order[T cmp.Ordered] struct {
	err error
	out []T
}

func NewOrder[T cmp.Ordered]() core.Primitive {
	return &Order[T]{}
}

func (op *Order[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			ordered := slices.Clone(*(*[]T)(arriving))
			slices.Sort(ordered)
			op.out = ordered

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Order[T]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
