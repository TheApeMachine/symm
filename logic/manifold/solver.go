package manifold

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/system"
)

/*
WaveMode is one resident ω-lattice spectral mode: its lattice frequency,
complex spectral-head coefficient, and gate linewidth.
*/
type WaveMode struct {
	Omega     float32
	Real      float32
	Imag      float32
	Linewidth float32
}

/*
State is everything Step returns for one advance of the resident domain: the
per-particle sensorium.State and Reading, the packed Eulerian gas/wave grid
fields, and the resident spectral mode lattice. Every field here is exactly
what sensorium.Manifold already exposes through State/Reading/PackFields/
SpectralModes/Grid — Step gathers all of it once per advance instead of a
separate caller re-deriving any of it later.
*/
type State struct {
	sensorium.State
	sensorium.Reading

	GridX, GridY, GridZ int
	GridSpacing         float64

	// MomRho is four floats per grid cell: momentum xyz then density.
	MomRho []float32
	// FieldEnergy is the packed Eulerian energy field, one value per grid
	// cell — distinct from the embedded sensorium.State.Energy, which is
	// per-particle.
	FieldEnergy []float32
	// WaveReal and WaveImag are the complex spatial wave field, one value per
	// grid cell.
	WaveReal []float32
	WaveImag []float32
	// DensityScale, MomentumScale, EnergyScale, and WaveScale are the peak
	// magnitudes PackFields observed while packing MomRho/FieldEnergy/
	// WaveReal/WaveImag, for normalizing a renderer's color/volume mapping.
	DensityScale  float32
	MomentumScale float32
	EnergyScale   float32
	WaveScale     float32

	Modes []WaveMode
}

/*
Solver owns one resident Sensorium domain for the complete market universe.
Symbols contribute orders to the same gas and wave fields; they are not split
into independent simulations that cannot interfere.

The solver subscribes to the dedicated Hawkes channel, so its ring carries only
the raw Hawkes measurements (the forcing term) — never every signal's converted
measurement. Each Hawkes observation projects the resting orders into
oscillators via the Dataset, loads them into the resident domain, and advances
the field once.
*/
type Solver struct {
	ctx           context.Context
	cancel        context.CancelFunc
	mu            sync.Mutex
	err           error
	workspace     *runtime.Workspace
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

func NewSolver(
	ctx context.Context, workspace *runtime.Workspace,
) *Solver {
	ctx, cancel := context.WithCancel(ctx)

	solver := &Solver{
		ctx:       ctx,
		cancel:    cancel,
		workspace: workspace,
		physics: sensorium.NewManifold(
			system.Cfg.Manifold.Grid.X,
			system.Cfg.Manifold.Grid.Y,
			system.Cfg.Manifold.Grid.Z,
			NewDataset(workspace),
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

	if workspace != nil {
		runtime.Register(
			workspace,
			nil,
			func(measurement *hawkes.Measurement) *State {
				if measurement == nil {
					return nil
				}

				return solver.Step(measurement.Measurement)
			},
		)
	}

	return solver
}

func (solver *Solver) Name() string { return "manifold" }

func (solver *Solver) Error() error { return solver.err }

/*
Step advances the resident field once. It is fired by a Hawkes measurement:
Hawkes is the forcing term, so each observation triggers a load-and-step of the
resident domain.
*/
func (solver *Solver) Step(measurement *data.Measurement[float64]) *State {
	if measurement == nil || solver.physics == nil {
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
