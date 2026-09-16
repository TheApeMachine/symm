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
func (meanShift MeanShift) Bound() float64 {
	if meanShift.RecentCount <= 0 || meanShift.PriorCount <= 0 || meanShift.Observations <= 0 || meanShift.Variance <= 0 {
		return 0
	}

	return math.Sqrt(meanShift.Variance * (math.Log(4*meanShift.Observations*meanShift.Observations) * (0.5 * (1/meanShift.RecentCount + 1/meanShift.PriorCount))))
}
