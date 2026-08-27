package manifold

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

type State struct {
	sensorium.State
	sensorium.Reading
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

	// sequence numbers the fluid slabs; it advances under mu on each publish.
	sequence uint64

	// fieldBuffers are reused across publishes so the 4 Hz field slab does not
	// churn a few megabytes of floats per tick.
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

	if workspace != nil {
		workspace.Wire(
			types.ChannelHawkes,
			"",
			func(value any) any {
				if m, ok := value.(*data.Measurement[float64]); ok && m != nil {
					_ = solver.Step(m)
				}

				return nil
			},
		)
	}

	if solver.physics != nil {
		gridX, gridY, gridZ, _ := solver.physics.Grid()
		cells := gridX * gridY * gridZ
		solver.fieldMomRho = make([]float32, cells*4)
		solver.fieldEnergy = make([]float32, cells)
		solver.fieldWaveReal = make([]float32, cells)
		solver.fieldWaveImag = make([]float32, cells)
	}

	go solver.publishLoop()

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

	return &State{
		State:   *state,
		Reading: solver.physics.Reading(),
	}
}

/*
fluidPublishInterval is the cadence of the fluid WebRTC publication loop.
*/
const fluidPublishInterval = 10 * time.Millisecond

/*
publishLoop streams the resident domain to the fluid channels on a fixed
cadence, independent of the Hawkes forcing cadence: the physics state persists
between steps, so the viewer stays live even when the forcing is sparse.
*/
func (solver *Solver) publishLoop() {
	ticker := time.NewTicker(fluidPublishInterval)
	defer ticker.Stop()

	for {
		select {
		case <-solver.ctx.Done():
			return
		case <-ticker.C:
			solver.publishFluid()
		}
	}
}

/*
publishFluid ships the latest resident state as three slabs: the Eulerian
fields, the oscillator gas, and the phase reading with its spectral modes.
The FluidRTC wire drops the frames when no viewer owns the channel, so the
encoding cost is only paid by viewers, never by the trading hot path.
*/
func (solver *Solver) publishFluid() {
	if solver.workspace == nil || solver.physics == nil {
		return
	}

	solver.mu.Lock()
	defer solver.mu.Unlock()

	state := solver.physics.State()

	if state == nil || state.N == 0 {
		return
	}

	solver.sequence++
	sequence := solver.sequence

	gridX, gridY, gridZ, gridSpacing := solver.physics.Grid()
	density, momentum, energyPeak, wave := solver.physics.PackFields(
		solver.fieldMomRho,
		solver.fieldEnergy,
		solver.fieldWaveReal,
		solver.fieldWaveImag,
	)

	fieldsSlab := encodeFieldsSlab(
		sequence,
		gridX, gridY, gridZ,
		float32(gridSpacing),
		density, momentum, energyPeak, wave,
		solver.fieldMomRho,
		solver.fieldEnergy,
		solver.fieldWaveReal,
		solver.fieldWaveImag,
	)

	heatScale := maxAbs32(state.Heat)
	energyScale := maxAbs32(state.Energy)
	massScale := maxAbs32(state.Mass)

	particlesSlab := encodeParticlesSlab(
		sequence,
		state,
		heatScale, energyScale, massScale,
	)

	modeOmega, modeReal, modeImag, modeLinewidth := solver.physics.SpectralModes()
	phaseSlab := encodePhaseSlab(
		sequence,
		solver.physics.Reading(),
		state,
		modeOmega, modeReal, modeImag, modeLinewidth,
	)

	solver.workspace.Publish(types.ChannelFluid, types.FluidFrame{
		Channel: types.FluidFieldsChannel,
		Payload: fieldsSlab,
	})
	solver.workspace.Publish(types.ChannelFluid, types.FluidFrame{
		Channel: types.FluidParticlesChannel,
		Payload: particlesSlab,
	})
	solver.workspace.Publish(types.ChannelFluid, types.FluidFrame{
		Channel: types.FluidPhaseChannel,
		Payload: phaseSlab,
	})
}

func maxAbs32(values []float32) float32 {
	var peak float32

	for _, value := range values {
		abs := float32(math.Abs(float64(value)))

		if abs > peak {
			peak = abs
		}
	}

	return peak
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
