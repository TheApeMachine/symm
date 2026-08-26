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
	Pos        []float32
	Vel        []float32
	Clamped    []bool
	Dark       []bool
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
