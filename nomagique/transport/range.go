package transport

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Range enumerates [0, count) for each arriving count.
*/
type Range struct {
	err error
	out float64
}

func NewRange() core.Primitive {
	return &Range{}
}

func (op *Range) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			count := int(*(*float64)(arriving))

			for index := 0; index < count; index++ {
				op.out = float64(index)

				if !yield(unsafe.Pointer(&op.out)) {
					return
				}
			}
		}
	}
}

func (op *Range) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
