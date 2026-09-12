package probability

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
GeometricMean owns exp(mean(log x)).
*/
type GeometricMean struct {
	err   error
	count float64
	sum   float64
	out   float64
}

func NewGeometricMean() core.Primitive {
	return &GeometricMean{}
}

func (op *GeometricMean) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			val := *(*float64)(arriving)
			op.count++
			op.sum += math.Log(val)
			op.out = math.Exp(op.sum / op.count)

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *GeometricMean) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
