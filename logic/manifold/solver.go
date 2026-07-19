package manifold

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/signal/compute"
	"github.com/theapemachine/symm/types"
)

const (
	defaultGridResolution = 64
	defaultMaxModes       = 128
)

/*
HawkesSource exposes the latest Hawkes excitation outcome per symbol.
*/
type HawkesSource interface {
	Symbols() []string
	Outcome(symbol string) (excitation.Outcome, bool)
}

/*
Solver owns one shared Metal engine and per-symbol resident Fields, advancing
the market-grounded L3 carrier field under Hawkes-derived temporal controls.
*/
type Solver struct {
	config   pmanifold.Config
	engine   *pmanifold.Engine
	symbols  map[string]*symbolSlot
	books    BookSource
	recorder *audit.Recorder
}

/*
SetRecorder attaches the runtime audit stream so forcing rescale and advance
failures leave a durable breadcrumb without blocking the GPU hot path.
*/
func (solver *Solver) SetRecorder(recorder *audit.Recorder) {
	if solver == nil {
		return
	}

	solver.recorder = recorder
}

type symbolSlot struct {
	handle *pmanifold.Solver
	epoch  uint64
	at     time.Time
	events int
	coords *coordinateEpoch
	// last is the most recent GasReady projection. Thesis is rebuilt every Cut,
	// so a quiet Hawkes outcome must republish this instead of vanishing from
	// the UI between trade arrivals.
	last State
}

/*
NewSolver constructs the shared GPU manifold engine from market configuration.
*/
func NewSolver(books BookSource) (*Solver, error) {
	grid := uint32(defaultGridResolution)

	deltaT := viper.GetDuration(
		"signals.fluid.integration_interval",
	)

	if deltaT <= 0 {
		idleThreshold := viper.GetDuration("signals.fluid.idle_threshold")
		maxIntegrationSteps := viper.GetInt("signals.fluid.max_integration_steps")

		if idleThreshold <= 0 || maxIntegrationSteps <= 0 {
			return nil, errnie.Err(
				errnie.Validation,
				"manifold: integration interval requires idle threshold and max steps",
				nil,
			)
		}

		deltaT = idleThreshold / time.Duration(maxIntegrationSteps)
	}

	config, err := pmanifold.NewConfig(
		grid,
		grid,
		grid,
		1,
		int(grid/2),
		deltaT.Seconds(),
		5.0/3.0,
		defaultMaxModes,
	)

	if err != nil {
		return nil, errnie.Err(
			errnie.Internal,
			"manifold: invalid engine configuration",
			err,
		)
	}

	pmanifold.DefaultMarketGasBoundaries().Apply(&config)

	var engine *pmanifold.Engine

	err = compute.WithMetalInit(func() error {
		created, createErr := pmanifold.NewEngine(config)

		if createErr != nil {
			return createErr
		}

		engine = created

		return nil
	})

	if err != nil {
		return nil, errnie.Err(
			errnie.Internal,
			"manifold: failed to create shared Metal engine",
			err,
		)
	}

	return &Solver{
		config:  config,
		engine:  engine,
		symbols: make(map[string]*symbolSlot),
		books:   books,
	}, nil
}

/*
Update advances the GPU field for every booked symbol whose observed arrival
intensity moved forward and stores the resulting readout on the thesis. A
fitted Hawkes kernel refines the forcing but is not required to start the
physical field. There is no resident-symbol ceiling.
*/
func (solver *Solver) Update(
	thesis *types.Thesis,
	hawkes HawkesSource,
) error {
	if solver == nil || thesis == nil || hawkes == nil {
		return nil
	}

	candidates := make([]intensityCandidate, 0, len(hawkes.Symbols()))

	for _, symbol := range hawkes.Symbols() {
		outcome, ok := hawkes.Outcome(symbol)

		if !ok || !outcome.Readiness.Intensity {
			continue
		}

		buyIntensity, sellIntensity := intensities(outcome)
		intensity := buyIntensity + sellIntensity

		if intensity <= 0 {
			continue
		}

		candidates = append(candidates, intensityCandidate{
			symbol:    symbol,
			outcome:   outcome,
			intensity: intensity,
		})
	}

	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].intensity == candidates[right].intensity {
			return candidates[left].symbol < candidates[right].symbol
		}

		return candidates[left].intensity > candidates[right].intensity
	})

	candidates = bookReady(candidates, solver.books)

	errnie.Error(audit.Record(solver.recorder, "manifold_select", map[string]any{
		"selected": len(candidates),
	}))

	var failures []error
	advanced := 0

	for _, candidate := range candidates {
		state, err := solver.advance(candidate.symbol, candidate.outcome)

		if err != nil {
			cause := err

			for errors.Unwrap(cause) != nil {
				cause = errors.Unwrap(cause)
			}

			failures = append(failures, errnie.Err(
				errnie.Internal,
				fmt.Sprintf("manifold: %s failed to advance field", candidate.symbol),
				err,
			).With("cause", cause.Error()))

			errnie.Error(audit.Record(solver.recorder, "manifold_advance", map[string]any{
				"symbol": candidate.symbol,
				"ok":     false,
				"error":  cause.Error(),
			}))

			continue
		}

		if !state.GasReady() {
			continue
		}

		if slot := solver.symbols[candidate.symbol]; slot != nil {
			slot.last = state
		}

		advanced++
		thesis.Manifold.Store(candidate.symbol, state)
	}

	errnie.Error(audit.Record(solver.recorder, "manifold", map[string]any{
		"candidates": len(candidates),
		"advanced":   advanced,
		"failed":     len(failures),
	}))

	return errors.Join(failures...)
}

/*
Close releases every resident Field, then the shared engine token.
*/
func (solver *Solver) Close() {
	if solver == nil {
		return
	}

	for symbol, slot := range solver.symbols {
		if slot != nil && slot.handle != nil {
			slot.handle.Close()
		}

		delete(solver.symbols, symbol)
	}

	if solver.engine != nil {
		solver.engine.Close()
		solver.engine = nil
	}
}
