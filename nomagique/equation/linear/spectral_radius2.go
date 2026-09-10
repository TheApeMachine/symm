package linear

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
SpectralRadius2 owns the spectral radius of a 2-by-2 matrix. For a
complex-conjugate pair the common modulus is sqrt(|det|); otherwise the
largest absolute real root is selected.
*/
type SpectralRadius2 struct {
	core.Base[Matrix2, float64]
}

func NewSpectralRadius2() *SpectralRadius2 {
	return &SpectralRadius2{}
}

func (op *SpectralRadius2) Next(
	in iter.Seq[core.Primitive[Matrix2, Matrix2]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for arriving := range in {
			m := arriving.Read()
			trace := m.A + m.D
			det := m.A*m.D - m.B*m.C
			disc := trace*trace - 4*det

			if disc < 0 {
				if !yield(op.Carrier(math.Sqrt(det))) {
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

			if !yield(op.Carrier(radius)) {
				return
			}
		}
	}
}
