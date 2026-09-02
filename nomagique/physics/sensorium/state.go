package sensorium

import "sync"

/*
State is the resident particle TensorDict: one row per PIC oscillator.
*/
type State struct {
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
	// Amp is the oscillator's wave amplitude, the square root of its energy.
	// It is derived from Energy on every load and carried back out on store so
	// observers can render the true amplitude without re-deriving it.
	Amp     []float32
	Pos     []float32
	Vel     []float32
	Clamped []bool
	Dark    []bool
}

func newState(count int) *State {
	return newStateWithCapacity(count, count)
}

func newStateWithCapacity(count, capacity int) *State {
	return &State{
		N:          count,
		Bytes:      make([]int64, count, capacity),
		Seqs:       make([]int64, count, capacity),
		TokenIDs:   make([]int64, count, capacity),
		ContentIDs: make([]int64, count, capacity),
		Phase:      make([]float32, count, capacity),
		Omega:      make([]float32, count, capacity),
		Energy:     make([]float32, count, capacity),
		Mass:       make([]float32, count, capacity),
		Heat:       make([]float32, count, capacity),
		Amp:        make([]float32, count, capacity),
		Pos:        make([]float32, count*3, capacity*3),
		Vel:        make([]float32, count*3, capacity*3),
		Clamped:    make([]bool, count, capacity),
		Dark:       make([]bool, count, capacity),
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
		state.Amp[index] = state.Amp[last]
		state.Clamped[index] = state.Clamped[last]
		state.Dark[index] = state.Dark[last]

		for axis := 0; axis < 3; axis++ {
			state.Pos[index*3+axis] = state.Pos[last*3+axis]
			state.Vel[index*3+axis] = state.Vel[last*3+axis]
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
	state.Amp = state.Amp[:last]
	state.Pos = state.Pos[:last*3]
	state.Vel = state.Vel[:last*3]
	state.Clamped = state.Clamped[:last]
	state.Dark = state.Dark[:last]
	state.N = last
}
