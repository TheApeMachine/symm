package transport

import (
	"errors"
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
	err   error
	right iter.Seq[unsafe.Pointer]
}

/*
NewZip2 instantiates a Zip2 Primitive holding the right run.
*/
func NewZip2[T any](right iter.Seq[unsafe.Pointer]) core.Primitive {
	return &Zip2[T]{right: right}
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

			pair := [2]T{*(*T)(arriving), *(*T)(other)}

			if !yield(unsafe.Pointer(&pair)) {
				return
			}
		}
	}
}

func (op *Zip2[T]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
