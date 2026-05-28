package fluid

import "github.com/theapemachine/symm/numeric"

/*
robustCrossSectionActivity maps a raw cross-section slice to a dimensionless
activity index: median / max(MAD, |median|·ε, floor). Div, Reynolds, vorticity,
and shock terms live on different raw scales; this puts them on one axis.
*/
func robustCrossSectionActivity(values []float64) float64 {
	return numeric.NewRobustScaler().Activity(values)
}
