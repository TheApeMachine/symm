package sensorium

import "sync"

/*
State is the resident particle TensorDict: one row per PIC oscillator.
*/
type State struct {
	// Positions at the preceding spectral Hamiltonian evaluation. Their
	// identity follows each particle, allowing exact potential-change partition.
	CoherencePosition []float32

	N          int
	Bytes      []int64
	Seqs       []int64
	TokenIDs   []int64
	ContentIDs []int64
	Phase      []float32
	Omega      []float32
	Energy     []float32
	Mass       []float32
	Heat       []float32
	// Independent conservative material energy (thermal + kinetic), paired with Heat.
	// Nil is the one-time migration state; it is never reconstructed every step.
	MaterialEnergy []float32
	// Amp is the oscillator's wave amplitude, the square root of its energy.
	// It is derived from Energy on every load and carried back out on store so
	// observers can render the true amplitude without re-deriving it.
	Amp []float32
	Pos []float32
	// Vel is TOTAL material transport velocity. PilotVel remembers the externally
	// prescribed pilot component, so a steady guidance field is not re-added as
	// a fresh velocity kick on every step.
	Vel      []float32
	PilotVel []float32
	// Last effective phase potential for explicit parameter/reseeding work.
	PhasePotential []float32
	Clamped        []bool
	Dark           []bool
}

func newState(count int) *State {
	return newStateWithCapacity(count, count)
}

func newStateWithCapacity(count, capacity int) *State {
	return &State{
		N:              count,
		Bytes:          make([]int64, count, capacity),
		Seqs:           make([]int64, count, capacity),
		TokenIDs:       make([]int64, count, capacity),
		ContentIDs:     make([]int64, count, capacity),
		Phase:          make([]float32, count, capacity),
		Omega:          make([]float32, count, capacity),
		Energy:         make([]float32, count, capacity),
		Mass:           make([]float32, count, capacity),
		Heat:           make([]float32, count, capacity),
		Amp:            make([]float32, count, capacity),
		Pos:            make([]float32, count*3, capacity*3),
		Vel:            make([]float32, count*3, capacity*3),
		PilotVel:       make([]float32, count*3, capacity*3),
		PhasePotential: make([]float32, count, capacity),
		Clamped:        make([]bool, count, capacity),
		Dark:           make([]bool, count, capacity),
	}
}

var StatePool = sync.Pool{
	New: func() any {
		return newState(1)
	},
}

func (state *State) empty() bool {
	return state == nil || state.N == 0
}

/*
append copies one row of incoming onto the end of state, growing it by a single
particle. It is how an order the resident domain has not seen before enters the
population without disturbing the particles already evolving in it.
*/
func (state *State) append(incoming *State, index int) {
	state.ensureCoherencePosition()
	incoming.ensureCoherencePosition()
	state.CoherencePosition = append(state.CoherencePosition, incoming.CoherencePosition[3*index:3*index+3]...)
	state.ensureMaterialEnergy()
	state.MaterialEnergy = append(state.MaterialEnergy, incoming.materialEnergyAt(index))
	state.Bytes = append(state.Bytes, incoming.Bytes[index])
	state.Seqs = append(state.Seqs, incoming.Seqs[index])
	state.TokenIDs = append(state.TokenIDs, incoming.TokenIDs[index])
	state.ContentIDs = append(state.ContentIDs, incoming.ContentIDs[index])
	state.Phase = append(state.Phase, incoming.Phase[index])
	state.Omega = append(state.Omega, incoming.Omega[index])
	state.Energy = append(state.Energy, incoming.Energy[index])
	state.Mass = append(state.Mass, incoming.Mass[index])
	state.Heat = append(state.Heat, incoming.Heat[index])
	state.Amp = append(state.Amp, incoming.Amp[index])

	state.Pos = append(state.Pos,
		incoming.Pos[index*3+0], incoming.Pos[index*3+1], incoming.Pos[index*3+2],
	)

	state.Vel = append(state.Vel,
		incoming.Vel[index*3+0], incoming.Vel[index*3+1], incoming.Vel[index*3+2],
	)

	if len(state.PilotVel) != 0 && len(state.PilotVel) != state.N*3 {
		panic("sensorium: partial PilotVel metadata")
	}

	if len(state.PilotVel) == 0 && state.N > 0 {
		state.PilotVel = make([]float32, state.N*3)
	}

	if len(incoming.PilotVel) >= index*3+3 {
		state.PilotVel = append(state.PilotVel, incoming.PilotVel[index*3:index*3+3]...)
	} else {
		state.PilotVel = append(state.PilotVel, 0, 0, 0)
	}

	if len(state.PhasePotential) != 0 && len(state.PhasePotential) != state.N {
		panic("sensorium: partial PhasePotential metadata")
	}

	if len(state.PhasePotential) == 0 && state.N > 0 {
		state.PhasePotential = make([]float32, state.N)
	}

	prior := float32(0)

	if len(incoming.PhasePotential) > index {
		prior = incoming.PhasePotential[index]
	}

	state.PhasePotential = append(state.PhasePotential, prior)
	state.Clamped = append(state.Clamped, incoming.Clamped[index])
	state.Dark = append(state.Dark, incoming.Dark[index])
	state.N++
}

/*
refresh updates a resident particle from a fresh observation of the same order.

Only what the venue just re-stated is taken: the order's rank-derived phase, its
price-derived frequency, and the energy/mass/heat the projection assigns it.
Position and velocity are deliberately left alone — those are the domain's own
integrated state, and overwriting them with the projection's seed coordinates
would restart the particle's trajectory on every book update.
*/
func (state *State) refresh(resident int, incoming *State, index int) {
	state.ensureMaterialEnergy()
	delta := float64(incoming.Heat[index]) - float64(state.Heat[resident])

	for a := range 3 {
		v := float64(state.Vel[3*resident+a])
		delta += .5 * (float64(incoming.Mass[index]) - float64(state.Mass[resident])) * v * v
	}

	state.MaterialEnergy[resident] = float32(float64(state.MaterialEnergy[resident]) + delta)
	state.Bytes[resident] = incoming.Bytes[index]
	state.Seqs[resident] = incoming.Seqs[index]
	state.TokenIDs[resident] = incoming.TokenIDs[index]
	state.Phase[resident] = incoming.Phase[index]
	state.Omega[resident] = incoming.Omega[index]
	state.Energy[resident] = incoming.Energy[index]
	state.Mass[resident] = incoming.Mass[index]
	state.Heat[resident] = incoming.Heat[index]
	state.Clamped[resident] = incoming.Clamped[index]
	state.Dark[resident] = incoming.Dark[index]
}

/*
remove swaps one resident row with the final row and contracts every tensor.
The caller owns the ContentID-to-row index and must update the moved row there.
*/
func (state *State) remove(index int) {
	last := state.N - 1

	if index != last {
		state.Bytes[index] = state.Bytes[last]
		state.Seqs[index] = state.Seqs[last]
		state.TokenIDs[index] = state.TokenIDs[last]
		state.ContentIDs[index] = state.ContentIDs[last]
		state.Phase[index] = state.Phase[last]
		state.Omega[index] = state.Omega[last]
		state.Energy[index] = state.Energy[last]
		state.Mass[index] = state.Mass[last]
		state.Heat[index] = state.Heat[last]

		if len(state.MaterialEnergy) == state.N {
			state.MaterialEnergy[index] = state.MaterialEnergy[last]
		}

		state.Amp[index] = state.Amp[last]

		if len(state.PhasePotential) == state.N {
			state.PhasePotential[index] = state.PhasePotential[last]
		}

		state.Clamped[index] = state.Clamped[last]
		state.Dark[index] = state.Dark[last]

		for axis := range 3 {
			state.Pos[index*3+axis] = state.Pos[last*3+axis]

			if len(state.CoherencePosition) == 3*state.N {
				state.CoherencePosition[3*index+axis] = state.CoherencePosition[3*last+axis]
			}

			state.Vel[index*3+axis] = state.Vel[last*3+axis]

			if len(state.PilotVel) == state.N*3 {
				state.PilotVel[index*3+axis] = state.PilotVel[last*3+axis]
			}
		}
	}

	state.Bytes = state.Bytes[:last]
	state.Seqs = state.Seqs[:last]
	state.TokenIDs = state.TokenIDs[:last]
	state.ContentIDs = state.ContentIDs[:last]
	state.Phase = state.Phase[:last]
	state.Omega = state.Omega[:last]
	state.Energy = state.Energy[:last]
	state.Mass = state.Mass[:last]
	state.Heat = state.Heat[:last]

	if len(state.MaterialEnergy) == state.N {
		state.MaterialEnergy = state.MaterialEnergy[:last]
	}

	state.Amp = state.Amp[:last]

	if len(state.PhasePotential) == state.N {
		state.PhasePotential = state.PhasePotential[:last]
	}

	if len(state.CoherencePosition) == 3*state.N {
		state.CoherencePosition = state.CoherencePosition[:last*3]
	}

	state.Pos = state.Pos[:last*3]
	state.Vel = state.Vel[:last*3]

	if len(state.PilotVel) == state.N*3 {
		state.PilotVel = state.PilotVel[:last*3]
	}

	state.Clamped = state.Clamped[:last]
	state.Dark = state.Dark[:last]
	state.N = last
}

func (state *State) materialEnergyAt(i int) float32 {
	if len(state.MaterialEnergy) == state.N {
		return state.MaterialEnergy[i]
	}

	if len(state.MaterialEnergy) != 0 {
		panic("sensorium: partial MaterialEnergy metadata")
	}

	value := float64(state.Heat[i])

	for a := range 3 {
		v := float64(state.Vel[3*i+a])
		value += .5 * float64(state.Mass[i]) * v * v
	}

	return float32(value)
}
func (state *State) ensureMaterialEnergy() {
	if len(state.MaterialEnergy) == state.N {
		return
	}

	if len(state.MaterialEnergy) != 0 {
		panic("sensorium: partial MaterialEnergy metadata")
	}

	values := make([]float32, state.N)

	for i := range values {
		values[i] = state.materialEnergyAt(i)
	}

	state.MaterialEnergy = values
}

func (state *State) ensureCoherencePosition() {
	if len(state.CoherencePosition) == state.N*3 {
		return
	}

	if len(state.CoherencePosition) != 0 {
		panic("sensorium: partial CoherencePosition metadata")
	}

	state.CoherencePosition = append([]float32(nil), state.Pos...)
}
