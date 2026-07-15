package manifold

import (
	"math"
	"time"

	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

/*
State is the typed physical readout produced after one Hawkes-driven GPU step.
*/
type State struct {
	Source           string               `json:"source"`
	Symbol           string               `json:"symbol"`
	At               time.Time            `json:"at"`
	Duration         time.Duration        `json:"duration"`
	Epoch            uint64               `json:"epoch"`
	ReferencePrice   float64              `json:"referencePrice"`
	Spread           float64              `json:"spread"`
	BuyCapacity      float64              `json:"buyCapacity"`
	SellCapacity     float64              `json:"sellCapacity"`
	InvalidReason    string               `json:"invalidReason,omitempty"`
	StressAnisotropy float64              `json:"stressAnisotropy"`
	Subdivisions     uint32               `json:"subdivisions"`
	BuyIntensity     float64              `json:"buyIntensity"`
	SellIntensity    float64              `json:"sellIntensity"`
	SpectralRadius   float64              `json:"spectralRadius"`
	Reading          pmanifold.Reading    `json:"reading"`
	OscillatorCount  int                  `json:"oscillatorCount"`
	Grid             pmanifold.Grid       `json:"grid,omitempty"`
	Rho              [][]float64          `json:"rho,omitempty"`
	PsiMag2          [][]float64          `json:"psiMag2,omitempty"`
	GuidanceVelX     [][]float64          `json:"guidanceVelX,omitempty"`
	GuidanceVelZ     [][]float64          `json:"guidanceVelZ,omitempty"`
	Particles        []pmanifold.Particle `json:"particles,omitempty"`
}

/*
IsFinite reports whether every numerical field required by downstream models
is a finite real number.
*/
func (state State) IsFinite() bool {
	if state.At.IsZero() || state.Epoch == 0 {
		return false
	}

	for _, value := range []float64{
		state.StressAnisotropy,
		state.ReferencePrice,
		state.Spread,
		state.BuyCapacity,
		state.SellCapacity,
		state.BuyIntensity,
		state.SellIntensity,
		state.SpectralRadius,
		state.Reading.PressureGradX,
		state.Reading.Divergence,
		state.Reading.CoherenceMag2,
		state.Reading.GuidanceSpeed,
	} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}

	return state.ReferencePrice > 0 && state.Spread > 0 &&
		state.BuyCapacity > 0 && state.SellCapacity > 0 &&
		state.Reading.IsFinite()
}

/*
GasReady reports whether the GPU gas step produced a finite physical readout.
*/
func (state State) GasReady() bool {
	return state.InvalidReason == Valid && state.IsFinite()
}
