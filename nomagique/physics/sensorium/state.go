package sensorium

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
	return &State{
		N:          count,
		Bytes:      make([]int64, count),
		Seqs:       make([]int64, count),
		TokenIDs:   make([]int64, count),
		ContentIDs: make([]int64, count),
		Phase:      make([]float32, count),
		Omega:      make([]float32, count),
		Energy:     make([]float32, count),
		Mass:       make([]float32, count),
		Heat:       make([]float32, count),
		Pos:        make([]float32, count*3),
		Vel:        make([]float32, count*3),
		Clamped:    make([]bool, count),
		Dark:       make([]bool, count),
	}
}

func (state *State) empty() bool {
	return state == nil || state.N == 0
}
