package transport

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Window exposes fixed-width overlapping groups within a delivery run. Width and
stride are structural configuration. Width 2 / stride 1 supplies consecutive
pairs; width N / stride N supplies disjoint groups. Groups are handed over as
soon as they are complete.
*/
type Window[T any] struct {
	core.Base[T, []T]
	width  int
	stride int
}

func NewWindow[T any](width, stride int) *Window[T] {
	op := &Window[T]{width: width, stride: stride}

	if width < 1 || stride < 1 {
		op.Error(core.ErrShape)
	}

	return op
}

func (op *Window[T]) Next(
	in iter.Seq[core.Primitive[T, T]],
) iter.Seq[core.Primitive[[]T, []T]] {
	return func(yield func(core.Primitive[[]T, []T]) bool) {
		if op.Error() != nil {
			return
		}

		var buf []T

		for arriving := range in {
			buf = append(buf, arriving.Read())

			if len(buf) < op.width {
				continue
			}

			group := append([]T(nil), buf[:op.width]...)

			if !yield(op.Carrier(group)) {
				return
			}

			drop := op.stride

			if drop > len(buf) {
				drop = len(buf)
			}

			buf = append(buf[:0], buf[drop:]...)
		}
	}
}
