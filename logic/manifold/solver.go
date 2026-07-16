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
Solver owns per-symbol GPU manifold slots and advances the market-grounded L3
carrier field under Hawkes-derived temporal controls.
*/
type Solver struct {
	config   pmanifold.Config
	capacity int
	symbols  map[string]*symbolSlot
	books    BookSource
}

type symbolSlot struct {
	handle *pmanifold.Solver
	epoch  uint64
	at     time.Time
	events int
	coords *coordinateEpoch
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
	capacity := viper.GetInt("market.manifold_max_symbols")

	if capacity <= 0 {
		return nil, errnie.Err(
			errnie.Validation,
			"manifold: symbol capacity must be positive",
			nil,
		)
	}

	return &Solver{
		config:   config,
		capacity: capacity,
		symbols:  make(map[string]*symbolSlot),
		books:    books,
	}, nil
}

/*
Update advances the GPU field for every symbol whose observed arrival intensity
moved forward and stores the resulting readout on the thesis. A fitted Hawkes
kernel refines the forcing but is not required to start the physical field. The
configured GPU capacity retains the strongest current absolute forcing.
*/
func (solver *Solver) Update(
	thesis *types.Thesis,
	hawkes HawkesSource,
) error {
	if solver == nil || thesis == nil || hawkes == nil {
		return nil
	}

	type candidate struct {
		symbol    string
		outcome   excitation.Outcome
		intensity float64
	}

	candidates := make([]candidate, 0, solver.capacity)

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

		candidates = append(candidates, candidate{
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

	if len(candidates) > solver.capacity {
		focusSymbol, _ := thesis.UIProjection()

		for index := solver.capacity; index < len(candidates); index++ {
			if candidates[index].symbol != focusSymbol {
				continue
			}

			candidates[solver.capacity-1] = candidates[index]
			break
		}

		candidates = candidates[:solver.capacity]
	}

	var failures []error

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

			continue
		}

		if !state.GasReady() {
			continue
		}

		thesis.Manifold.Store(candidate.symbol, state)
	}

	return errors.Join(failures...)
}

func (solver *Solver) advance(
	symbol string,
	outcome excitation.Outcome,
) (State, error) {
	slot := solver.symbols[symbol]

	if slot != nil && (outcome.At.Before(slot.at) ||
		(outcome.At.Equal(slot.at) && outcome.EventCount == slot.events)) {
		return State{}, nil
	}

	var prior *coordinateEpoch

	if slot != nil {
		prior = slot.coords
	}

	oscillators, coordEpoch, ready, err := solver.populationOscillators(
		symbol, outcome.At, prior,
	)

	if err != nil {
		return State{}, err
	}

	if !ready {
		return State{}, nil
	}

	if slot == nil || slot.handle == nil {
		slot, err = solver.admit(symbol)

		if err != nil {
			return State{}, err
		}
	}

	handle := slot.handle
	interval := eventInterval(solver.config, slot.at, outcome)
	slot.at = outcome.At
	slot.events = outcome.EventCount
	characteristicSpeed, err := applyForcing(
		solver.config,
		outcome,
		interval,
		oscillators,
	)

	if err != nil {
		return State{}, err
	}

	controls := runtimeControls(
		solver.config,
		outcome,
		interval,
	)
	advectiveDeltaT := solver.config.AdvectiveDeltaT(characteristicSpeed)

	if advectiveDeltaT <= 0 {
		return State{}, errnie.Err(
			errnie.Validation,
			"manifold: carrier state produced no stable integration step",
			nil,
		)
	}

	if advectiveDeltaT < controls.DeltaT {
		controls.DeltaT = advectiveDeltaT
	}

	if err := handle.SetControls(controls); err != nil {
		return State{}, err
	}

	if err := inject(
		handle,
		oscillators,
	); err != nil {
		return State{}, err
	}

	reading, err := handle.Step()

	if err != nil {
		handle.Close()
		slot.handle = nil

		return State{}, err
	}

	slot.coords = coordEpoch
	slot.epoch++
	buyIntensity, sellIntensity := intensities(outcome)

	projection, projectionErr := projectField(handle, solver.config, len(oscillators))

	if projectionErr != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("manifold: %s failed to read field projection", symbol),
			projectionErr,
		))
	}

	return State{
		Source:           "manifold",
		Symbol:           symbol,
		At:               outcome.At,
		Duration:         interval,
		Epoch:            slot.epoch,
		ReferencePrice:   coordEpoch.midPrice,
		Spread:           coordEpoch.spread,
		BuyCapacity:      coordEpoch.buyCapacity,
		SellCapacity:     coordEpoch.sellCapacity,
		InvalidReason:    Valid,
		StressAnisotropy: stressAnisotropy(outcome),
		Subdivisions:     1,
		BuyIntensity:     buyIntensity,
		SellIntensity:    sellIntensity,
		SpectralRadius:   outcome.Fit.SpectralRadius,
		Reading:          reading,
		OscillatorCount:  len(oscillators),
		Grid:             projection.Grid,
		Rho:              projection.Rho,
		PsiMag2:          projection.PsiMag2,
		GuidanceVelX:     projection.GuidanceVelX,
		GuidanceVelZ:     projection.GuidanceVelZ,
		Particles:        projection.Particles,
	}, nil
}

/*
admit allocates a GPU field for the requested symbol, replacing the least
recently advanced field when the configured working-set capacity is full.
*/
func (solver *Solver) admit(symbol string) (*symbolSlot, error) {
	if symbol == "" {
		return nil, errnie.Err(
			errnie.Internal,
			"manifold: symbol is required",
			nil,
		)
	}

	slot, ok := solver.symbols[symbol]

	if ok && slot.handle != nil {
		return slot, nil
	}

	active := 0

	for _, candidateSlot := range solver.symbols {
		if candidateSlot.handle != nil {
			active++
		}
	}

	if active >= solver.capacity {
		var oldestSlot *symbolSlot

		for _, candidateSlot := range solver.symbols {
			if candidateSlot.handle == nil {
				continue
			}

			if oldestSlot == nil || candidateSlot.at.Before(oldestSlot.at) {
				oldestSlot = candidateSlot
			}
		}

		oldestSlot.handle.Close()
		oldestSlot.handle = nil
	}

	err := compute.WithMetalInit(func() error {
		handle, err := pmanifold.NewSolver(solver.config)

		if err != nil {
			return err
		}

		if slot == nil {
			slot = &symbolSlot{}
			solver.symbols[symbol] = slot
		}

		slot.handle = handle

		return nil
	})

	if err != nil {
		return nil, errnie.Err(
			errnie.Internal,
			"manifold: failed to initialize solver",
			err,
		)
	}

	return slot, nil
}

func (solver *Solver) populationOscillators(
	symbol string,
	at time.Time,
	prior *coordinateEpoch,
) ([]pmanifold.Oscillator, *coordinateEpoch, bool, error) {
	if solver.books == nil {
		return nil, prior, false, errnie.Err(
			errnie.Internal,
			"manifold: L3 book source is not configured",
			nil,
		)
	}

	orders, midPrice, ok := ordersForSymbol(solver.books, symbol)

	if !ok {
		return nil, prior, false, nil
	}

	mapped, coordEpoch, ready := mapOrders(solver.config, orders, midPrice, at, prior)

	if !ready {
		return nil, prior, false, nil
	}

	oscillators := cohortsFromMappedOrders(solver.config, mapped)

	if len(oscillators) == 0 {
		return nil, prior, false, nil
	}

	return oscillators, coordEpoch, true, nil
}

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
}
