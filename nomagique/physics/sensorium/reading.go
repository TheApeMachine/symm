package sensorium

import "math"

/*
Reading is one observational snapshot of the resident gas and ω-field.
*/
type Reading struct {
	Divergence       float64
	GuidanceSpeed    float64
	CoherenceMag2    float64
	PressureGradNorm float64
	ViscosityProxy   float64
	KuramotoR        float64
}

func (reading Reading) IsFinite() bool {
	return finite(reading.Divergence) &&
		finite(reading.GuidanceSpeed) &&
		finite(reading.CoherenceMag2) &&
		finite(reading.PressureGradNorm) &&
		finite(reading.ViscosityProxy) &&
		finite(reading.KuramotoR)
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
