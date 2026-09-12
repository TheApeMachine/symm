package statistic

import (
	"errors"
	"iter"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Median owns the central order statistic of one run. It averages the two central
members. Empty runs report ErrShape.
*/
type Median struct {
	err error
	out float64
}

func NewMedian() core.Primitive {
	return &Median{}
}

func (op *Median) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values []float64

		for arriving := range in {
			values = append(values, *(*float64)(arriving))
		}

		if len(values) == 0 {
			op.err = errors.Join(op.err, core.ErrShape)
			return
		}

		slices.Sort(values)
		count := len(values)
		op.out = (values[(count-1)/2] + values[count/2]) / 2

		yield(unsafe.Pointer(&op.out))
	}
}

func (op *Median) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
