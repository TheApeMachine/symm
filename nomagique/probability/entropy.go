package probability

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Entropy owns -sum(p log p). Zero mass contributes its limiting value zero;
negative inputs retain the logarithm's undefined-domain result.
*/
type Entropy struct {
	err error
	acc float64
	out float64
}

func NewEntropy() core.Primitive {
	return &Entropy{}
}

func (op *Entropy) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			mass := *(*float64)(arriving)
			contribution := 0.0

			if mass != 0 {
				contribution = -mass * math.Log(mass)
			}

			op.acc += contribution
			op.out = op.acc

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Entropy) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
