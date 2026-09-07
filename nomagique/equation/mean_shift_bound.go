package equation

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
	"math"
)

/*
MeanShift holds the existing all/recent window cut's numeric inputs. The bound
is the existing approximation, not a claim of full ADWIN equivalence.
*/
type MeanShift struct{ Variance, Observations, RecentCount, PriorCount float64 }

/* Bound uses ln(4*n*n) and the two-subwindow reciprocal support sum. */
func (shift MeanShift) Bound() float64 {
	return math.Sqrt(shift.Variance * (math.Log(4*shift.Observations*shift.Observations) * (0.5 * (1/shift.RecentCount + 1/shift.PriorCount))))
}

/* MeanShiftBound projects named inputs into the canonical typed cut. */
type MeanShiftBound struct {
	core.PrimitiveError
	seed    *transport.IO
	current core.Primitive
}

func NewMeanShiftBound() core.Primitive {
	return &MeanShiftBound{seed: transport.NewIO(core.From(0.0))}
}
func (bound *MeanShiftBound) Next(input core.Primitive) core.Primitive {
	result := core.Yield(bound.seed, input, func(_ float64, fields map[string]core.Primitive) float64 {
		decoder := core.NewDecoder(fields)
		shift := MeanShift{
			Variance: core.Decode[float64](decoder, "variance"), Observations: core.Decode[float64](decoder, "observations"),
			RecentCount: core.Decode[float64](decoder, "recent_count"), PriorCount: core.Decode[float64](decoder, "prior_count"),
		}
		bound.Error(decoder.Error())
		return shift.Bound()
	}, bound)

	if result != nil {
		bound.current = result
	}
	return result
}
func (bound *MeanShiftBound) Read() any { return core.To[any](bound.current) }
