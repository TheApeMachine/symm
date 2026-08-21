package manifold

import (
	"math"

	pmanifold "github.com/theapemachine/symm/nomagique/physics/sensorium"
)

/*
Oscillator is one Level 3 order as a PIC particle / phase oscillator.
Amplitude is the mass deposited; energy is Amplitude².
*/
type Oscillator struct {
	Phase     float64
	Omega     float64
	Amplitude float64
	PosX      float64
	PosY      float64
	PosZ      float64
	Heat      float64
	VelX      float64
	VelY      float64
	VelZ      float64
}

func oscillatorsToState(oscillators []Oscillator) *pmanifold.State {
	state := &pmanifold.State{
		N:      len(oscillators),
		Phase:  make([]float32, len(oscillators)),
		Omega:  make([]float32, len(oscillators)),
		Energy: make([]float32, len(oscillators)),
		Mass:   make([]float32, len(oscillators)),
		Heat:   make([]float32, len(oscillators)),
		Pos:    make([]float32, len(oscillators)*3),
		Vel:    make([]float32, len(oscillators)*3),
	}

	for index, oscillator := range oscillators {
		state.Phase[index] = float32(oscillator.Phase)
		state.Omega[index] = float32(oscillator.Omega)
		state.Energy[index] = float32(oscillator.Amplitude * oscillator.Amplitude)
		state.Mass[index] = float32(oscillator.Amplitude)
		state.Heat[index] = float32(oscillator.Heat)
		state.Pos[index*3+0] = float32(oscillator.PosX)
		state.Pos[index*3+1] = float32(oscillator.PosY)
		state.Pos[index*3+2] = float32(oscillator.PosZ)
		state.Vel[index*3+0] = float32(oscillator.VelX)
		state.Vel[index*3+1] = float32(oscillator.VelY)
		state.Vel[index*3+2] = float32(oscillator.VelZ)
	}

	return state
}

func stateToOscillators(state *pmanifold.State) []Oscillator {
	if state == nil || state.N == 0 {
		return nil
	}

	oscillators := make([]Oscillator, state.N)

	for index := 0; index < state.N; index++ {
		oscillators[index] = Oscillator{
			Phase:     float64(state.Phase[index]),
			Omega:     float64(state.Omega[index]),
			Amplitude: math.Sqrt(float64(state.Energy[index])),
			PosX:      float64(state.Pos[index*3+0]),
			PosY:      float64(state.Pos[index*3+1]),
			PosZ:      float64(state.Pos[index*3+2]),
			Heat:      float64(state.Heat[index]),
			VelX:      float64(state.Vel[index*3+0]),
			VelY:      float64(state.Vel[index*3+1]),
			VelZ:      float64(state.Vel[index*3+2]),
		}
	}

	return oscillators
}
