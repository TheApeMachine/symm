package matrix

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
SpectralRadius2 owns the spectral radius of a 2-by-2 matrix. Each arrival is
*[4]float64 {a, b, c, d}, row-major; it yields *float64.
*/
type SpectralRadius2 struct {
	*core.PrimitiveError
	out float64
}

func NewSpectralRadius2() core.Primitive {
	return &SpectralRadius2{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *SpectralRadius2) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := (*[4]float64)(arriving)
			trace := m[0] + m[3]
			det := m[0]*m[3] - m[1]*m[2]
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
