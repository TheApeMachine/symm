package matrix

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
SpectralRadius2 owns the spectral radius of a 2-by-2 matrix.
*/
type SpectralRadius2 struct {
	err error
	out float64
}

func NewSpectralRadius2() core.Primitive {
	return &SpectralRadius2{}
}

func (op *SpectralRadius2) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(*Matrix2)(arriving)
			trace := m.A + m.D
			det := m.A*m.D - m.B*m.C
			disc := trace*trace - 4*det

			if disc < 0 {
				op.out = math.Sqrt(det)

				if !yield(unsafe.Pointer(&op.out)) {
					return
				}

				continue
			}

			root := math.Sqrt(disc)
			left := math.Abs((trace + root) / 2)
			right := math.Abs((trace - root) / 2)
			radius := left

			if right > radius {
				radius = right
			}

			op.out = radius

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *SpectralRadius2) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
