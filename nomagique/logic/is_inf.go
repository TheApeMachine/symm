package logic

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
IsInf owns the infinity predicate. It reports whether an arrival is infinite,
irrespective of sign.
*/
type IsInf struct {
	err error
	out bool
}

func NewIsInf() core.Primitive {
	return &IsInf{}
}

func (op *IsInf) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			op.out = math.IsInf(*in, 0)

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *IsInf) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
