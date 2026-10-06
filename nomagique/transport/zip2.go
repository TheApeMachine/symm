package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Zip2 pairs corresponding yields of the same type from the inbound left run
and the right run held at construction, as [2]T. It stops when either run
ends.
*/
type Zip2[T any] struct {
	*core.PrimitiveError
	right iter.Seq[unsafe.Pointer]
	out   [2]T
}

/*
NewZip2 instantiates a Zip2 Primitive holding the right run.
*/
func NewZip2[T any](right iter.Seq[unsafe.Pointer]) core.Primitive {
	return &Zip2[T]{
		PrimitiveError: core.NewPrimitiveError(),
		right:          right,
	}
}

func (op *Zip2[T]) Next(left iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		next, stop := iter.Pull(op.right)
		defer stop()

		for arriving := range left {
			other, ok := next()

			if !ok {
				return
			}

			op.out = [2]T{*(*T)(arriving), *(*T)(other)}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
