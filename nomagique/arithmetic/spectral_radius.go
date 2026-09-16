package arithmetic

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
SpectralRadius2 owns the spectral radius of a 2-by-2 matrix.
*/
type SpectralRadius2 struct {
	*core.PrimitiveError

	out float64
}

func NewSpectralRadius2() *SpectralRadius2 {
	return &SpectralRadius2{PrimitiveError: core.NewPrimitiveError()}
}

func (spectralRadius2 *SpectralRadius2) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(*Matrix2)(arriving)
			trace := m.A + m.D
			det := m.A*m.D - m.B*m.C
			disc := trace*trace - 4*det

			if disc < 0 {
				spectralRadius2.out = math.Sqrt(det)

				if !yield(unsafe.Pointer(&spectralRadius2.out)) {
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

			spectralRadius2.out = radius

			if !yield(unsafe.Pointer(&spectralRadius2.out)) {
				return
			}
		}
	}
}
