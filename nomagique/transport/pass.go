package transport

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Pass hands each arrival over unchanged. It is the identity stage.
*/
type Pass struct {
	err error
}

func NewPass() core.Primitive {
	return &Pass{}
}

func (op *Pass) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Pass) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
