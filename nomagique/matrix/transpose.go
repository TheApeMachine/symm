package matrix

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Transpose changes only matrix addressing. Ragged rows are a shape error, not
silently padded zeros.
*/
type Transpose[T any] struct {
	err error
	out [][]T
}

func NewTranspose[T any]() core.Primitive {
	return &Transpose[T]{}
}

func (op *Transpose[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			rows := *(*[][]T)(arriving)

			if len(rows) == 0 {
				op.out = [][]T{}

				if !yield(unsafe.Pointer(&op.out)) {
					return
				}

				continue
			}

			width := len(rows[0])
			columns := make([][]T, width)

			for column := range columns {
				columns[column] = make([]T, len(rows))
			}

			ok := true

			for row, values := range rows {
				if len(values) != width {
					op.Error(core.ErrShape)
					ok = false
					break
				}

				for column, value := range values {
					columns[column][row] = value
				}
			}

			if !ok {
				return
			}

			op.out = columns

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Transpose[T]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
