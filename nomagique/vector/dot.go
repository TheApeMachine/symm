package vector

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Dot owns the inner product of two equal-length vectors.
*/
type Dot struct {
	err error
	out float64
}

func NewDot() core.Primitive {
	return &Dot{}
}

func (op *Dot) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			pair := (*Pair)(arriving)

			if len(pair.Left) != len(pair.Right) {
				op.Error(core.ErrShape)
				continue
			}

			op.out = 0.0
			for index, value := range pair.Left {
				op.out += value * pair.Right[index]
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Dot) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
