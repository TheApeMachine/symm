package fluid

import (
	"math"

	"github.com/theapemachine/symm/numeric"
)

/*
robustCrossSectionActivity maps a raw cross-section slice to a dimensionless
activity index: median / max(MAD, |median|·ε, floor). Div, Reynolds, vorticity,
and shock terms live on different raw scales; this puts them on one axis.
*/
func robustCrossSectionActivity(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := numeric.CopySorted(values)
	median := numeric.PercentileSorted(sorted, 0.5)
	mad := numeric.MedianAbsoluteDeviation(sorted, median)
	spread := math.Max(mad, math.Max(math.Abs(median)*1e-3, 1e-12))

	if mad <= 0 {
		spread = math.Max(math.Abs(median)*0.1, 1e-12)
	}

	return median / spread
}
