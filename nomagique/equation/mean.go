package equation

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Mean owns the running arithmetic mean. Configuration supplies the value a run
starts from; the observation count is owned here because the mean is not a
mean without it.
*/
type Mean[U core.Floating] struct {
	core.Base[U, U]
	count U
}

func NewMean[U core.Floating]() *Mean[U] {
	return &Mean[U]{}
}

func (op *Mean[U]) Write(value U) {
	op.Base.Write(value)
	op.count = 0
}

func (op *Mean[U]) Next(
	in iter.Seq[core.Primitive[U, U]],
) iter.Seq[core.Primitive[U, U]] {
	return func(yield func(core.Primitive[U, U]) bool) {
		for arriving := range in {
			op.count++
			held := op.Read()

			if !yield(op.Carrier(held + (arriving.Read()-held)/op.count)) {
				return
			}
		}
	}
}
