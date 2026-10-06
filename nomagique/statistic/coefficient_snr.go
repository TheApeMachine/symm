package statistic

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
CoefficientSNR owns the primary coefficient SNR, Coefficient² / Variance, as a
Primitive. It is non-negative and unbounded. It is not probability or
confidence, and it is undefined (NaN) when the coefficient variance is
unavailable or zero.
*/
type CoefficientSNR struct {
	*core.PrimitiveError
	out float64
}

/*
NewCoefficientSNR instantiates the coefficient signal-to-noise Primitive.
*/
func NewCoefficientSNR() *CoefficientSNR {
	return &CoefficientSNR{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

/*
Next scores every arriving *[2]float64 {coefficient, variance} and hands over
the SNR.
*/
func (op *CoefficientSNR) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			pair := (*[2]float64)(arriving)
			coefficient, variance := pair[0], pair[1]
			op.out = math.NaN()

			if !math.IsNaN(variance) && !math.IsInf(variance, 0) && variance > 0 {
				op.out = coefficient * coefficient / variance
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
