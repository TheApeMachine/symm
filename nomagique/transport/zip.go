package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Zip pairs corresponding yields from the inbound left run and the right run
held at construction. Each yield is *[2]any{left, right}. It stops when
either run ends.
*/
type Zip[T, U any] struct {
	*core.PrimitiveError
	right iter.Seq[unsafe.Pointer]
	out   [2]any
}

/*
NewZip instantiates a Zip Primitive holding the right run.
*/
func NewZip[T, U any](right iter.Seq[unsafe.Pointer]) core.Primitive {
	return &Zip[T, U]{
		PrimitiveError: core.NewPrimitiveError(),
		right:          right,
	}
}

func (op *Zip[T, U]) Next(left iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		next, stop := iter.Pull(op.right)
		defer stop()

		for arriving := range left {
			other, ok := next()

			if !ok {
				return
			}

			op.out = [2]any{*(*T)(arriving), *(*U)(other)}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
