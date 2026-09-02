package sensorium

import (
	"fmt"
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
	// resident maps a particle's ContentID to its row in state, so an
	// incremental batch can find the row an already-resident order is
	// evolving in instead of rebuilding the population.
	resident  map[int64]int
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
Remove evicts explicitly departed particles from the resident domain.
Absence from an incremental batch is not a departure; callers must provide the
ContentIDs named by the authoritative source lifecycle.
*/
func (manifold *Manifold) Remove(contentIDs []int64) (int, error) {
	if manifold == nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Internal,
			"manifold: physics domain is not initialized",
			nil,
		))
	}

	manifold.mu.Lock()
	defer manifold.mu.Unlock()
	remaining := 0

	if manifold.state != nil {
		remaining = manifold.state.N
	}

	if len(contentIDs) == 0 {
		return remaining, nil
	}

	if manifold.state == nil {
		return 0, errnie.Error(errnie.Err(
			errnie.NotFound,
			"manifold: cannot remove a particle from an empty domain",
			nil,
		))
	}

	if manifold.resident == nil {
		manifold.resident = make(map[int64]int, manifold.state.N)

		for index := 0; index < manifold.state.N; index++ {
			manifold.resident[manifold.state.ContentIDs[index]] = index
		}
	}

	departures := make(map[int64]struct{}, len(contentIDs))

	for _, contentID := range contentIDs {
		if _, duplicate := departures[contentID]; duplicate {
			return manifold.state.N, errnie.Error(errnie.Err(
				errnie.Conflict,
				fmt.Sprintf("manifold: duplicate departure content_id=%d", contentID),
				nil,
			))
		}

		if _, resident := manifold.resident[contentID]; !resident {
			return manifold.state.N, errnie.Error(errnie.Err(
				errnie.NotFound,
				fmt.Sprintf("manifold: departed content_id=%d is not resident", contentID),
				nil,
			))
		}

		departures[contentID] = struct{}{}
	}

	for _, contentID := range contentIDs {
		index := manifold.resident[contentID]
		last := manifold.state.N - 1
		movedContentID := manifold.state.ContentIDs[last]
		manifold.state.remove(index)
		delete(manifold.resident, contentID)

		if index != last {
			manifold.resident[movedContentID] = index
		}
	}

	return manifold.state.N, nil
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
	reading, err := manifold.work.step()

	if err != nil {
		return nil, err
	}

	manifold.reading = reading
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

/*
merge folds an incoming batch into the resident domain by ContentID, which is
the stable per-order identity the projection stamps on every particle.

The domain is resident: a batch is what this message observed, never the whole
population. An order already resident keeps the row it has been evolving — only
the freshly observed quantities are refreshed, so its integrated position and
velocity survive the update — while an order seen for the first time is
appended. Replacing the population with the batch instead made the field
collapse to whatever the last message happened to carry, which for an
incremental Level3 update is a single order.

Departure is the book's business, not the batch's: a message that simply does
not mention an order says nothing about whether it is still resting, so nothing
is evicted here.
*/
func (manifold *Manifold) merge(incoming *State) {
	if incoming == nil || incoming.empty() {
		return
	}

	if manifold.state.empty() {
		manifold.state = incoming
		manifold.resident = make(map[int64]int, incoming.N)

		for index := 0; index < incoming.N; index++ {
			manifold.resident[incoming.ContentIDs[index]] = index
		}

		return
	}

	if manifold.resident == nil {
		manifold.resident = make(map[int64]int, manifold.state.N)

		for index := 0; index < manifold.state.N; index++ {
			manifold.resident[manifold.state.ContentIDs[index]] = index
		}
	}

	for index := 0; index < incoming.N; index++ {
		contentID := incoming.ContentIDs[index]
		resident, seen := manifold.resident[contentID]

		if !seen {
			manifold.state.append(incoming, index)
			manifold.resident[contentID] = manifold.state.N - 1
			continue
		}

		manifold.state.refresh(resident, incoming, index)
	}
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
