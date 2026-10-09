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

/*
Next consumes one run whole: the run commits to the running total only when
every arrival is a value, and then yields that total once. A malformed run
records ErrShape and commits nothing.
*/
func (op *Sum) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		total := op.total
		observed := false

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			total += *(*float64)(arriving)
			observed = true
		}

		if !observed {
			return
		}

		op.total = total
		op.out = total
		yield(unsafe.Pointer(&op.out))
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
