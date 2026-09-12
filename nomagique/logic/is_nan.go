package logic

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
IsNaN owns the undefinedness predicate. It reports whether an arrival is NaN;
it does not replace, skip, or otherwise keep invalid state alive.
*/
type IsNaN struct {
	err error
	out bool
}

func NewIsNaN() core.Primitive {
	return &IsNaN{}
}

func (op *IsNaN) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			op.out = math.IsNaN(*in)

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *IsNaN) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
