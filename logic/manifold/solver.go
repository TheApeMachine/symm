package manifold

import (
	"context"
	"sync"
	"time"

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

It satisfies nomagique/runtime.Node[*types.Envelope] and plays two roles
depending on which envelope it sees: a Level3 envelope folds into Dataset's
own accumulated order book (no field advance, no output); a Trade envelope
carrying a Hawkes measurement — the forcing term — projects that book into
oscillators via Dataset, loads them into the resident domain, and advances the
field once, writing the result onto the envelope.
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
			dataset,
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
Step folds a Level3 envelope into Dataset's accumulated book (no output), or
advances the resident field once for a Trade envelope carrying a Hawkes
measurement — the forcing term — writing the resulting State onto the
envelope. Any other envelope is a no-op.
*/
func (solver *Solver) Step(envelope *types.Envelope) *types.Envelope {
	switch envelope.TypeID {
	case types.EnvelopeLevel3:
		solver.dataset.Step(envelope.Level3Data)
	case types.EnvelopeTrade:
		if envelope.Hawkes != nil {
			envelope.Manifold = solver.advance()
		}
	}

	return envelope
}

/*
advance loads the current resident book into the field and advances it once.
It is fired by a Hawkes measurement: Hawkes is the forcing term, so each
observation triggers a load-and-step of the resident domain.
*/
func (solver *Solver) advance() *State {
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

	if err := solver.physics.Load(); err != nil {
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
