package statistic

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Sum owns the running arithmetic sum.
*/
type Sum struct {
	err   error
	total float64
	out   float64
}

func NewSum() core.Primitive {
	return &Sum{}
}

func (op *Sum) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			val := *(*float64)(arriving)
			op.total += val
			op.out = op.total

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Sum) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
