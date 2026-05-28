package numeric

import "math"

const (
	robustRelativeFloor = 1e-3
	robustAbsoluteFloor = 1e-12
	robustZeroMADRatio  = 0.1
)

/*
RobustScaler maps arbitrary numeric vectors onto a median/MAD activity axis.
*/
type RobustScaler struct{}

/*
NewRobustScaler returns a stateless robust scaler.
*/
func NewRobustScaler() RobustScaler {
	return RobustScaler{}
}

/*
Activity returns median / max(MAD, relative floor, absolute floor).
*/
func (robustScaler RobustScaler) Activity(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := CopySorted(values)
	median := PercentileSorted(sorted, 0.5)
	mad := MedianAbsoluteDeviation(sorted, median)
	spread := math.Max(
		mad,
		math.Max(math.Abs(median)*robustRelativeFloor, robustAbsoluteFloor),
	)

	if mad <= 0 {
		spread = math.Max(math.Abs(median)*robustZeroMADRatio, robustAbsoluteFloor)
	}

	return median / spread
}
