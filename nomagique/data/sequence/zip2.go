package sequence

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
}

/*
NewZip2 instantiates a Zip2 Primitive holding the right run.
*/
func NewZip2[T any](right iter.Seq[unsafe.Pointer]) *Zip2[T] {
	return &Zip2[T]{PrimitiveError: core.NewPrimitiveError(), right: right}
}

func (zip2 *Zip2[T]) Next(left iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		next, stop := iter.Pull(zip2.right)
		defer stop()

		for arriving := range left {
			other, ok := next()

			if !ok {
				return
			}

			pair := [2]T{*(*T)(arriving), *(*T)(other)}

			if !yield(unsafe.Pointer(&pair)) {
				return
			}
		}
	}
}
