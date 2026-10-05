package statistic

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
CoefficientSNRPair bundles one estimated coefficient and its estimated variance.
*/
type CoefficientSNRPair struct {
	Coefficient float64
	Variance    float64
}

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
Next scores every arriving coefficient/variance pair and hands over the SNR.
*/
func (op *CoefficientSNR) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			pair := (*CoefficientSNRPair)(arriving)
			op.out = coefficientSNR(pair.Coefficient, pair.Variance)

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
coefficientSNR returns Coefficient² / Variance, undefined (NaN) when the
coefficient variance is unavailable or zero.
*/
func coefficientSNR(coefficient float64, variance float64) float64 {
	if math.IsNaN(variance) || math.IsInf(variance, 0) || variance <= 0 {
		return math.NaN()
	}

	return coefficient * coefficient / variance
}
