package manifold

import (
	"context"
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
