package matrix

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Matrix2 is the four entries of a 2-by-2 matrix.
*/
type Matrix2 struct {
	A, B, C, D float64
}

/*
Determinant2 owns ad − bc.
*/
type Determinant2 struct {
	err error
	out float64
}

func NewDeterminant2() core.Primitive {
	return &Determinant2{}
}

func (op *Determinant2) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(*Matrix2)(arriving)
			op.out = m.A*m.D - m.B*m.C

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Determinant2) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
