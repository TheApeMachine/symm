package vector

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Difference subtracts paired members. Unequal lengths are a shape error.
*/
type Difference struct {
	err error
	out []float64
}

func NewDifference() core.Primitive {
	return &Difference{}
}

func (op *Difference) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			pair := (*Pair)(arriving)

			if len(pair.Left) != len(pair.Right) {
				op.Error(core.ErrShape)
				continue
			}

			if len(op.out) != len(pair.Left) {
				op.out = make([]float64, len(pair.Left))
			}

			for index, value := range pair.Left {
				op.out[index] = value - pair.Right[index]
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Difference) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
