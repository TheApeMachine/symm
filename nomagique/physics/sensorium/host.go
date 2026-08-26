package sensorium

import (
	"sync"

	"github.com/theapemachine/errnie"
)

/*
Manifold is the original host: tokenize, merge a particle State, then
thermo.step + wave.step.
*/
type Manifold struct {
	mu        sync.Mutex
	work      *workspace
	Tokenizer *Tokenizer
	state     *State
	reading   Reading
	stepCount int
}

func NewManifold(gridX, gridY, gridZ int, datasets ...Dataset) *Manifold {
	tokenizer, err := NewTokenizer(gridX, gridY, gridZ, 64, datasets...)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"sensorium: failed to get tokenizer",
			err,
		))

		return nil
	}

	work, err := newWorkspace(gridX, gridY, gridZ)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"sensorium: failed to get workspace",
			err,
		))

		return nil
	}

	return &Manifold{
		work:      work,
		Tokenizer: tokenizer,
		state:     &State{},
	}
}

func (manifold *Manifold) Close() {
	if manifold == nil {
		return
	}

	manifold.mu.Lock()
	defer manifold.mu.Unlock()

	if manifold.work != nil {
		manifold.work.Close()
		manifold.work = nil
	}
}

func (manifold *Manifold) State() *State {
	if manifold == nil {
		return nil
	}

	manifold.mu.Lock()
	defer manifold.mu.Unlock()

	return manifold.state
}

func (manifold *Manifold) Reading() Reading {
	if manifold == nil {
		return Reading{}
	}

	manifold.mu.Lock()
	defer manifold.mu.Unlock()

	return manifold.reading
}

func (manifold *Manifold) Grid() (gridX, gridY, gridZ int, spacing float64) {
	if manifold == nil || manifold.work == nil {
		return 0, 0, 0, 0
	}

	grid := manifold.work.domain
	return grid.GridX, grid.GridY, grid.GridZ, grid.GridSpacing()
}

/*
SpectralModes returns the resident ω-lattice mode spectrum: the aggregated
spectral-head coefficients (meanHeads output) paired with each mode's lattice
frequency and gate linewidth. Copies are returned so callers can render the
spectrum without aliasing the GPU-backed buffers.
*/
func (manifold *Manifold) SpectralModes() (omega, real, imag, linewidth []float32) {
	if manifold == nil || manifold.work == nil {
		return nil, nil, nil, nil
	}

	manifold.mu.Lock()
	defer manifold.mu.Unlock()

	manifold.work.engine.Synchronize()
	modes := int(manifold.work.domain.MaxModes)

	omega = make([]float32, modes)
	real = make([]float32, modes)
	imag = make([]float32, modes)
	linewidth = make([]float32, modes)

	copy(omega, manifold.work.omegaLattice.Float32Slice())
	copy(real, manifold.work.psiModeReal.Float32Slice())
	copy(imag, manifold.work.psiModeImag.Float32Slice())
	copy(linewidth, manifold.work.gateWidth.Float32Slice())

	return omega, real, imag, linewidth
}

func (manifold *Manifold) DomainExtent() (x, y, z float64) {
	if manifold == nil || manifold.work == nil {
		return 0, 0, 0
	}

	grid := manifold.work.domain
	return grid.DomainX, grid.DomainY, grid.DomainZ
}

/*
AddBatch injects particles. Different batch length replaces resident state
(original append_batches=false).
*/
func (manifold *Manifold) AddBatch(incoming *State) {
	if manifold == nil {
		return
	}

	manifold.mu.Lock()
	defer manifold.mu.Unlock()

	manifold.merge(incoming)
}

/*
Step merges an optional incoming batch then advances thermo + wave once.
*/
func (manifold *Manifold) Step(incoming *State) (*State, error) {
	if manifold == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"manifold: physics domain is not initialized",
			nil,
		))
	}

	manifold.mu.Lock()
	defer manifold.mu.Unlock()

	return manifold.stepLocked(incoming)
}

func (manifold *Manifold) stepLocked(incoming *State) (*State, error) {
	if manifold.work == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"manifold: physics domain is not initialized",
			nil,
		))
	}

	if incoming != nil {
		manifold.merge(incoming)
	}

	if manifold.state.empty() {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"manifold: no global or local state provided",
			nil,
		))
	}

	manifold.work.loadState(manifold.state)
	manifold.reading = manifold.work.step()
	manifold.work.storeState(manifold.state)
	manifold.stepCount++
	return manifold.state, nil
}

func (manifold *Manifold) PackFields(
	momRho, energy, waveReal, waveImag []float32,
) (density, momentum, energyPeak, wave float32) {
	if manifold == nil || manifold.work == nil {
		return 0, 0, 0, 0
	}

	manifold.mu.Lock()
	defer manifold.mu.Unlock()

	scale := manifold.work.packFields(momRho, energy, waveReal, waveImag)
	return scale.density, scale.momentum, scale.energy, scale.wave
}

func (manifold *Manifold) merge(incoming *State) {
	if incoming == nil || incoming.empty() {
		return
	}

	if manifold.state.empty() || manifold.state.N != incoming.N {
		manifold.state = incoming
		return
	}

	copy(manifold.state.Bytes, incoming.Bytes)
	copy(manifold.state.Seqs, incoming.Seqs)
	copy(manifold.state.TokenIDs, incoming.TokenIDs)
	copy(manifold.state.ContentIDs, incoming.ContentIDs)
	copy(manifold.state.Phase, incoming.Phase)
	copy(manifold.state.Omega, incoming.Omega)
	copy(manifold.state.Energy, incoming.Energy)
	copy(manifold.state.Mass, incoming.Mass)
	copy(manifold.state.Heat, incoming.Heat)
	copy(manifold.state.Amp, incoming.Amp)
	copy(manifold.state.Pos, incoming.Pos)
	copy(manifold.state.Vel, incoming.Vel)
	copy(manifold.state.Clamped, incoming.Clamped)
	copy(manifold.state.Dark, incoming.Dark)
}

/*
Load streams tokenizer datasets, stepping after each compressed batch.
*/
func (manifold *Manifold) Load() error {
	if manifold == nil || manifold.Tokenizer == nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"manifold: tokenizer required",
			nil,
		))
	}

	if len(manifold.Tokenizer.Datasets) == 0 {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"manifold: no datasets",
			nil,
		))
	}

	loader := NewLoader(manifold.Tokenizer)

	manifold.mu.Lock()
	defer manifold.mu.Unlock()

	for _, batch := range loader.Stream() {
		if _, err := manifold.stepLocked(batch); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"manifold: failed to step",
				err,
			))
		}
	}

	return nil
}
