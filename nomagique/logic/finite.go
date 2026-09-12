package logic

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Finite owns the finiteness predicate. What it hands over is whether each
arrival is a finite number.
*/
type Finite struct {
	err error
	out bool
}

func NewFinite() core.Primitive {
	return &Finite{}
}

func (op *Finite) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			in := (*float64)(arriving)
			number := *in
			op.out = !math.IsNaN(number) && !math.IsInf(number, 0)

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Finite) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
