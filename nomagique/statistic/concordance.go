package statistic

import (
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Concordance retains sufficient statistics of paired, dimensionless movements.
An inactive side contributes zero alignment, including an absent observation.
Both inactive sides supply no evidence. A consistent inverse pair has the same
strength as a consistent direct pair, but the opposite Orientation.

Each arrival is *[3]float64 {left, right, weight}, where weight is the clock
increment. It yields *[3]float64 {strength, orientation, support}.

The attraction is absolute mean sign agreement minus its standard error, plus
relative magnitude agreement supported by that sign agreement. Thus uncertain
or contradictory alignment can repel; no selected correlation cutoff is used.
*/
type Concordance struct {
	*core.PrimitiveError
	support       float64
	aligned       float64
	weightSquared float64
	mean          float64
	m2            float64
	magnitude     float64
	out           [3]float64
}

func NewConcordance() *Concordance {
	return &Concordance{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *Concordance) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			pair := (*[3]float64)(arriving)
			left, right, weight := pair[0], pair[1], pair[2]

			if weight < 0 {
				op.Error(fmt.Errorf("%w: concordance: negative clock increment", core.ErrDomain))
				return
			}

			prior := op.aligned
			alignment := 0.0
			active := left != 0 || right != 0

			if weight != 0 && active {
				if left != 0 && right != 0 {
					alignment = math.Copysign(1, left) * math.Copysign(1, right)
				}

				op.support += weight
				op.aligned += weight * alignment
				op.weightSquared += weight * weight
				delta := alignment - op.mean
				op.mean += weight * delta / op.support
				op.m2 += weight * delta * (alignment - op.mean)
				denominator := math.Abs(left) + math.Abs(right)
				op.magnitude += weight * (1 - math.Abs(math.Abs(left)-math.Abs(right))/denominator)
			}

			op.out = [3]float64{0, 0, op.support}

			if op.support != 0 {
				mean := op.aligned / op.support
				op.out[1] = math.Copysign(1, mean)
				consistency := math.Abs(mean)
				// E[(sign-mean)^2] includes unilateral movement as disagreement.
				effective := op.support * op.support / op.weightSquared
				variance := op.m2 / op.support
				uncertainty := math.Sqrt(variance / effective)
				op.out[0] = consistency - uncertainty + consistency*op.magnitude/op.support
			}

			if weight != 0 && active && prior != 0 && alignment != math.Copysign(1, prior) {
				op.out[0] = -math.Abs(op.out[0])
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
