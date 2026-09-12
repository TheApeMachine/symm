package calculus

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Convert owns one representation pass-through on the wire.
*/
type Convert struct {
	err error
}

func NewConvert() core.Primitive {
	return &Convert{}
}

func (op *Convert) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Convert) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
