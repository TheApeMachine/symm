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
Update advances the GPU field for the selected active set — open and pending
inventory, the UI focus symbol, then the highest-intensity booked candidates up
to the configured budget — and stores each readout on the thesis. A fitted
Hawkes kernel refines the forcing but is not required to start the field. Cold,
non-protected fields are evicted once the resident count exceeds the budget.
*/
func (solver *Solver) Update(
	thesis *types.Thesis,
	hawkes HawkesSource,
) error {
	if solver == nil || thesis == nil || hawkes == nil {
		return nil
	}

	candidates := solver.candidates(hawkes)
	active := newActiveSet(thesis)
	selected := active.selectAdvance(candidates)

	errnie.Error(audit.Record(solver.recorder, "manifold_select", map[string]any{
		"candidates": len(candidates),
		"selected":   len(selected),
		"budget":     active.budget,
	}))

	failures, advanced, latest := solver.advanceAll(thesis, selected)
	active.evict(solver, latest)

	errnie.Error(audit.Record(solver.recorder, "manifold", map[string]any{
		"candidates": len(candidates),
		"advanced":   advanced,
		"failed":     len(failures),
		"resident":   len(solver.symbols),
	}))

	return errors.Join(failures...)
}

/*
candidates builds the intensity-ranked, book-grounded candidate list for one
cut. Hawkes symbols without observed intensity or without a two-sided L3 touch
are dropped before any field is stepped.
*/
func (solver *Solver) candidates(hawkes HawkesSource) []intensityCandidate {
	symbols := hawkes.Symbols()
	candidates := make([]intensityCandidate, 0, len(symbols))

	for _, symbol := range symbols {
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

	return bookReady(candidates, solver.books)
}

/*
advanceAll steps each selected candidate, stores GasReady readouts on the
thesis, and returns the advance failures, the count advanced, and the latest
observed event time used to judge resident-field coldness during eviction.
*/
func (solver *Solver) advanceAll(
	thesis *types.Thesis,
	selected []intensityCandidate,
) ([]error, int, time.Time) {
	var failures []error

	advanced := 0
	latest := time.Time{}

	for _, candidate := range selected {
		if candidate.outcome.At.After(latest) {
			latest = candidate.outcome.At
		}

		state, err := solver.advance(candidate.symbol, candidate.outcome)

		if err != nil {
			failures = append(failures, solver.noteAdvanceFailure(candidate.symbol, err))
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

	return failures, advanced, latest
}

/*
noteAdvanceFailure unwraps one advance error to its root cause, records a
durable audit breadcrumb, and returns the annotated error for aggregation.
*/
func (solver *Solver) noteAdvanceFailure(symbol string, err error) error {
	cause := err

	for errors.Unwrap(cause) != nil {
		cause = errors.Unwrap(cause)
	}

	errnie.Error(audit.Record(solver.recorder, "manifold_advance", map[string]any{
		"symbol": symbol,
		"ok":     false,
		"error":  cause.Error(),
	}))

	return errnie.Err(
		errnie.Internal,
		fmt.Sprintf("manifold: %s failed to advance field", symbol),
		err,
	).With("cause", cause.Error())
}

/*
release closes and removes one resident field so an evicted symbol frees its GPU
allocation immediately.
*/
func (solver *Solver) release(symbol string) {
	slot := solver.symbols[symbol]

	if slot == nil {
		return
	}

	if slot.handle != nil {
		slot.handle.Close()
		slot.handle = nil
	}

	delete(solver.symbols, symbol)
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
			slot.handle = nil
		}

		delete(solver.symbols, symbol)
	}

	if solver.engine != nil {
		solver.engine.Close()
		solver.engine = nil
	}
}
