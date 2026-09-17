package correlation

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
CorrelationInput is a covariance and the two energies that normalize it.
*/
type CorrelationInput struct {
	Covariance  float64
	LeftEnergy  float64
	RightEnergy float64
}

/*
Correlation normalizes covariance by sqrt(left energy * right energy).
*/
type Correlation struct {
	*core.PrimitiveError
}

func NewCorrelation() *Correlation {
	return &Correlation{PrimitiveError: core.NewPrimitiveError()}
}

func (correlation *Correlation) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			estimate := (*LagEstimate)(arriving)
			out := *estimate
			scale := math.Sqrt(estimate.LeftEnergy * estimate.RightEnergy)

			if scale > 0 {
				out.Correlation = estimate.Covariance / scale
			}

			if !yield(unsafe.Pointer(&out)) {
				return
			}
		}
	}
}
