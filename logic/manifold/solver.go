package manifold

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

// WaveMode and State are defined in types so types.Envelope can carry a
// manifold advance without an import cycle back to logic/manifold.
type WaveMode = types.WaveMode
type State = types.ManifoldState

/*
Solver owns one resident Sensorium domain for the complete market universe.
Symbols contribute orders to the same gas and wave fields; they are not split
into independent simulations that cannot interfere.

It satisfies nomagique/runtime.Node[*types.Envelope]. A Level3 envelope projects
its message's orders into oscillators exactly once, loads them into the resident
domain, and advances the field once — every message, forward only. There is no
retained book and no separate trade trigger: the Level3 stream is the whole
input, and each message is one load-and-step.
*/
type Solver struct {
	ctx           context.Context
	cancel        context.CancelFunc
	mu            sync.Mutex
	err           error
	status        *runtime.Status
	dataset       *Dataset
	physics       *sensorium.Manifold
	ObserveModule func(string, time.Duration)

	// fieldMomRho/fieldEnergy/fieldWaveReal/fieldWaveImag are Step's private
	// working buffers for PackFields, reused across advances so gathering the
	// grid fields does not itself allocate; State.MomRho etc. is always a
	// fresh copy handed to the caller, never these buffers.
	fieldMomRho   []float32
	fieldEnergy   []float32
	fieldWaveReal []float32
	fieldWaveImag []float32
}

func NewSolver(ctx context.Context) *Solver {
	ctx, cancel := context.WithCancel(ctx)

	dataset := NewDataset()

	solver := &Solver{
		ctx:     ctx,
		cancel:  cancel,
		status:  runtime.NewStatus(),
		dataset: dataset,
		physics: sensorium.NewManifold(
			system.Cfg.Manifold.Grid.X,
			system.Cfg.Manifold.Grid.Y,
			system.Cfg.Manifold.Grid.Z,
		),
	}

	if solver.physics != nil {
		gridX, gridY, gridZ, _ := solver.physics.Grid()
		cells := gridX * gridY * gridZ
		solver.fieldMomRho = make([]float32, cells*4)
		solver.fieldEnergy = make([]float32, cells)
		solver.fieldWaveReal = make([]float32, cells)
		solver.fieldWaveImag = make([]float32, cells)
	}

	return solver
}

func (solver *Solver) Name() string { return "manifold" }

func (solver *Solver) Error() error { return solver.err }

/*
Step projects one Level3 message into a particle batch and advances the field
once. Any other envelope is a no-op.
*/
func (solver *Solver) Step(envelope *types.Envelope) *types.Envelope {
	if envelope.TypeID != types.EnvelopeLevel3 {
		return envelope
	}

	envelope.Manifold = solver.advance(envelope.Level3Data)

	return envelope
}

/*
advance projects one Level3 message's orders into a single batch, loads it into
the resident domain, and advances the field once.
*/
func (solver *Solver) advance(message kraken.Level3Data) *State {
	if solver.physics == nil {
		return nil
	}

	solver.mu.Lock()
	defer solver.mu.Unlock()

	started := time.Now()
	defer func() {
		if solver.ObserveModule != nil {
			solver.ObserveModule("manifold", time.Since(started))
		}
	}()

	batch := solver.project(message)

	if batch == nil || batch.N == 0 {
		return nil
	}

	if _, err := solver.physics.Step(batch); err != nil {
		solver.err = err
		return nil
	}

	state := solver.physics.State()

	if state == nil || state.N == 0 {
		return nil
	}

	gridX, gridY, gridZ, gridSpacing := solver.physics.Grid()
	densityScale, momentumScale, energyScale, waveScale := solver.physics.PackFields(
		solver.fieldMomRho,
		solver.fieldEnergy,
		solver.fieldWaveReal,
		solver.fieldWaveImag,
	)

	modeOmega, modeReal, modeImag, modeLinewidth := solver.physics.SpectralModes()
	modes := make([]WaveMode, len(modeOmega))

	for index := range modeOmega {
		modes[index] = WaveMode{
			Omega:     modeOmega[index],
			Real:      modeReal[index],
			Imag:      modeImag[index],
			Linewidth: modeLinewidth[index],
		}
	}

	return &State{
		State:   *state,
		Reading: solver.physics.Reading(),

		GridX: gridX, GridY: gridY, GridZ: gridZ, GridSpacing: gridSpacing,

		MomRho:      append([]float32(nil), solver.fieldMomRho...),
		FieldEnergy: append([]float32(nil), solver.fieldEnergy...),
		WaveReal:    append([]float32(nil), solver.fieldWaveReal...),
		WaveImag:    append([]float32(nil), solver.fieldWaveImag...),

		DensityScale:  densityScale,
		MomentumScale: momentumScale,
		EnergyScale:   energyScale,
		WaveScale:     waveScale,

		Modes: modes,
	}
}

/*
project folds one Level3 message's orders into a single State batch. Entries
are packed in arrival order with no sort; the batch is sized exactly to the
message so one message costs one forward pass and nothing is retained.
*/
func (solver *Solver) project(message kraken.Level3Data) *sensorium.State {
	count := len(message.Bids) + len(message.Asks)

	if count == 0 {
		return nil
	}

	batch := &sensorium.State{
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
		Amp:        make([]float32, count),
		Pos:        make([]float32, count*3),
		Vel:        make([]float32, count*3),
		Clamped:    make([]bool, count),
		Dark:       make([]bool, count),
	}

	index := 0

	for state := range solver.dataset.Step(message) {
		if state == nil || state.N != 1 {
			sensorium.StatePool.Put(state)
			continue
		}

		batch.Bytes[index] = state.Bytes[0]
		batch.Seqs[index] = state.Seqs[0]
		batch.TokenIDs[index] = state.TokenIDs[0]
		batch.ContentIDs[index] = state.ContentIDs[0]
		batch.Phase[index] = state.Phase[0]
		batch.Omega[index] = state.Omega[0]
		batch.Energy[index] = state.Energy[0]
		batch.Mass[index] = state.Mass[0]
		batch.Heat[index] = state.Heat[0]
		batch.Amp[index] = state.Amp[0]
		batch.Pos[index*3+0] = state.Pos[0]
		batch.Pos[index*3+1] = state.Pos[1]
		batch.Pos[index*3+2] = state.Pos[2]
		batch.Vel[index*3+0] = state.Vel[0]
		batch.Vel[index*3+1] = state.Vel[1]
		batch.Vel[index*3+2] = state.Vel[2]
		batch.Clamped[index] = state.Clamped[0]
		batch.Dark[index] = state.Dark[0]
		index++

		sensorium.StatePool.Put(state)
	}

	batch.N = index

	if batch.N == 0 {
		return nil
	}

	return batch
}

func (solver *Solver) Close() error {
	if solver == nil {
		return nil
	}

	solver.mu.Lock()
	defer solver.mu.Unlock()

	if solver.cancel != nil {
		solver.cancel()
	}

	if solver.physics != nil {
		solver.physics.Close()
		solver.physics = nil
	}

	return nil
}
