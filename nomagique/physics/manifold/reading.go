package manifold

import "math"

/*
Reading is the post-step observable bundle read back from the GPU manifold solver.

PressureGrad* are finite-difference estimates of ∇p on the torus.
CoherenceMag2 is the mean |Ψ|² across active ω-modes.
GuidanceSpeed is the mean |v| of carriers after pilot-wave gather — the Bohm current actually applied, not a mode-coefficient proxy.
ViscosityProxy is |∇·u|⁻¹ when divergence is non-zero — large when flow is laminar.
*/
type Reading struct {
	PressureGradX    float64 `json:"pressureGradX"`
	PressureGradY    float64 `json:"pressureGradY"`
	PressureGradZ    float64 `json:"pressureGradZ"`
	PressureGradNorm float64 `json:"pressureGradNorm"`
	Divergence       float64 `json:"divergence"`
	CoherenceMag2    float64 `json:"coherenceMag2"`
	GuidanceSpeed    float64 `json:"guidanceSpeed"`
	ViscosityProxy   float64 `json:"viscosityProxy"`
}

/*
IsFinite reports whether every observable in the bundle is a finite real number.
*/
func (reading Reading) IsFinite() bool {
	values := []float64{
		reading.PressureGradX,
		reading.PressureGradY,
		reading.PressureGradZ,
		reading.PressureGradNorm,
		reading.Divergence,
		reading.CoherenceMag2,
		reading.GuidanceSpeed,
		reading.ViscosityProxy,
	}

	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}

	return true
}
