package matrix

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Finite reports whether every coefficient is a finite number.
*/
type Finite struct {
	err error
	out bool
}

func NewFinite() core.Primitive {
	return &Finite{}
}

func (op *Finite) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			rows := *(*[][]float64)(arriving)
			valid := true

			for _, row := range rows {
				for _, value := range row {
					if math.IsNaN(value) || math.IsInf(value, 0) {
						valid = false
						break
					}
				}

				if !valid {
					break
				}
			}

			op.out = valid

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Finite) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
