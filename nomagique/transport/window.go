package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Window exposes fixed-width overlapping groups within a delivery run.
*/
type Window[T any] struct {
	*core.PrimitiveError
	width  int
	stride int
	out    []T
}

func NewWindow[T any](width, stride int) core.Primitive {
	op := &Window[T]{
		PrimitiveError: core.NewPrimitiveError(),
		width:          width,
		stride:         stride,
	}

	if width < 1 || stride < 1 {
		op.Error(core.ErrShape)
	}

	return op
}

func (op *Window[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if op.Error() != nil {
			return
		}

		var buf []T

		for arriving := range in {
			buf = append(buf, *(*T)(arriving))

			if len(buf) < op.width {
				continue
			}

			op.out = append([]T(nil), buf[:op.width]...)

			if !yield(unsafe.Pointer(&op.out)) {
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
