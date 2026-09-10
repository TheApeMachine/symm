package store

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Constant replaces each arrival with a configured value. The arrival is the
clock; the payload is ignored.
*/
type Constant[T, U any] struct {
	core.Base[T, U]
}

func NewConstant[T, U any](current U) *Constant[T, U] {
	op := &Constant[T, U]{}
	op.Carrier(current)
	return op
}

func (op *Constant[T, U]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for range in {
			if !yield(op.Carrier(op.Read())) {
				return
			}
		}
	}
}
