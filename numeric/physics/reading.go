package physics

import "math"

/*
Reading is the post-step observable bundle read back from the GPU manifold solver.

PressureGrad* are finite-difference estimates of ∇p on the torus.
CoherenceMag2 is the mean |Ψ|² across active ω-modes.
GuidanceSpeed is the mean pilot-wave velocity magnitude from the coherence field.
ViscosityProxy is |∇·u|⁻¹ when divergence is non-zero — large when flow is laminar.
*/
type Reading struct {
	PressureGradX    float64
	PressureGradY    float64
	PressureGradZ    float64
	PressureGradNorm float64
	Divergence       float64
	CoherenceMag2    float64
	GuidanceSpeed    float64
	ViscosityProxy   float64
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
