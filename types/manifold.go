package types

import (
	"time"

	"github.com/theapemachine/symm/nomagique/physics/sensorium"
)

type WaveMode struct {
	Omega     float32
	Real      float32
	Imag      float32
	Linewidth float32
}

type ManifoldState struct {
	At            time.Time
	Version       uint64
	State         sensorium.State
	Reading       sensorium.Reading
	GridX         int
	GridY         int
	GridZ         int
	GridSpacing   float64
	MomRho        []float32
	FieldEnergy   []float32
	WaveReal      []float32
	WaveImag      []float32
	DensityScale  float32
	MomentumScale float32
	EnergyScale   float32
	WaveScale     float32
	Modes         []WaveMode
}
