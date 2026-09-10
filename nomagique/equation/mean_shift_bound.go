package equation

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
MeanShift holds the existing all/recent window cut's numeric inputs. The bound
is the existing approximation, not a claim of full ADWIN equivalence.
*/
type MeanShift struct {
	Variance     float64
	Observations float64
	RecentCount  float64
	PriorCount   float64
}

/*
Bound uses ln(4 n n) and the two-subwindow reciprocal support sum.
*/
func (shift MeanShift) Bound() float64 {
	return math.Sqrt(shift.Variance * (math.Log(4*shift.Observations*shift.Observations) * (0.5 * (1/shift.RecentCount + 1/shift.PriorCount))))
}

/*
MeanShiftBound projects named inputs into the canonical typed cut.
*/
type MeanShiftBound struct {
	core.Base[MeanShift, float64]
}

func NewMeanShiftBound() *MeanShiftBound {
	return &MeanShiftBound{}
}

func (op *MeanShiftBound) Next(
	in iter.Seq[core.Primitive[MeanShift, MeanShift]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(arriving.Read().Bound())) {
				return
			}
		}
	}
}
