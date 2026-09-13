package statistic

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Mean owns the running arithmetic mean.
*/
type Mean struct {
	err   error
	count float64
	mean  float64
	out   float64
}

func NewMean() core.Primitive {
	return &Mean{}
}

func (op *Mean) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		count, mean := 0.0, 0.0

		for arriving := range in {
			val := *(*float64)(arriving)
			count++
			mean += (val - mean) / count
			op.out = mean

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Mean) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
