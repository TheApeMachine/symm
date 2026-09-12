package matrix

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Diagonal selects row i's member i. An undersized row is a shape error.
*/
type Diagonal struct {
	err error
	out []float64
}

func NewDiagonal() core.Primitive {
	return &Diagonal{}
}

func (op *Diagonal) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			rows := *(*[][]float64)(arriving)
			op.out = make([]float64, len(rows))
			ok := true

			for index, row := range rows {
				if index >= len(row) {
					op.Error(core.ErrShape)
					ok = false
					break
				}

				op.out[index] = row[index]
			}

			if !ok {
				return
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Diagonal) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
