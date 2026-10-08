package adaptive

import "math"

/*
MeanShift holds the all/recent window cut's numeric inputs.
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
	if shift.RecentCount <= 0 || shift.PriorCount <= 0 || shift.Observations <= 0 || shift.Variance <= 0 {
		return 0
	}

	return math.Sqrt(shift.Variance * (math.Log(4*shift.Observations*shift.Observations) * (0.5 * (1/shift.RecentCount + 1/shift.PriorCount))))
}
