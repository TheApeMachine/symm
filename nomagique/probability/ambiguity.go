package probability

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Ambiguity divides entropy by the entropy of an equal-mass distribution.
A one-member distribution has zero ambiguity by definition.
*/
type Ambiguity struct {
	err error
	out float64
}

func NewAmbiguity() core.Primitive {
	return &Ambiguity{}
}

func (op *Ambiguity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values []float64
		var total float64

		for arriving := range in {
			val := *(*float64)(arriving)
			values = append(values, val)
			total += val
		}

		if len(values) <= 1 {
			op.out = 0
			yield(unsafe.Pointer(&op.out))
			return
		}

		if total == 0 {
			op.out = 0
			yield(unsafe.Pointer(&op.out))
			return
		}

		entropy := 0.0

		for _, val := range values {
			p := val / total

			if p > 0 {
				entropy -= p * math.Log(p)
			}
		}

		op.out = entropy / math.Log(float64(len(values)))
		yield(unsafe.Pointer(&op.out))
	}
}

func (op *Ambiguity) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
