package manifold

import (
	"errors"
	"fmt"
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
Solver owns per-symbol GPU manifold slots and advances the toroidal fluid field
from Hawkes excitation outcomes.
*/
type Solver struct {
	config   pmanifold.Config
	capacity int
	symbols  map[string]*symbolSlot
	handle   *pmanifold.Solver
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
	).Seconds()

	if deltaT <= 0 {
		deltaT = 0.01
	}

	config, err := pmanifold.NewConfig(
		grid,
		grid,
		grid,
		1,
		int(grid/2),
		deltaT,
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
		capacity = 64
	}

	return &Solver{
		config:   config,
		capacity: capacity,
		symbols:  make(map[string]*symbolSlot),
		books:    books,
	}, nil
}

/*
Update advances the GPU field for every symbol whose Hawkes intensity moved
forward and stores the resulting readout on the thesis.
*/
func (solver *Solver) Update(
	thesis *types.Thesis,
	hawkes HawkesSource,
) {
	if solver == nil || thesis == nil || hawkes == nil {
		return
	}

	for _, symbol := range hawkes.Symbols() {
		outcome, ok := hawkes.Outcome(symbol)

		if !ok || !outcome.Readiness.HawkesFit {
			continue
		}

		buyIntensity, sellIntensity := intensities(outcome)

		if buyIntensity+sellIntensity <= 0 {
			continue
		}

		state, err := solver.advance(symbol, outcome)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				fmt.Sprintf("manifold: %s failed to advance field", symbol),
				err,
			))

			continue
		}

		if !state.GasReady() {
			continue
		}

		thesis.Manifold.Store(symbol, state)
	}
}

func (solver *Solver) advance(
	symbol string,
	outcome excitation.Outcome,
) (State, error) {
	slot, err := solver.admit(symbol)

	if err != nil {
		return State{}, err
	}

	if outcome.At.Before(slot.at) ||
		(outcome.At.Equal(slot.at) && outcome.EventCount == slot.events) {
		return State{}, nil
	}

	handle := slot.handle
	controls := runtimeControls(solver.config, outcome)

	if err := handle.SetControls(controls); err != nil {
		return State{}, err
	}

	oscillators, coordEpoch, err := solver.populationOscillators(symbol, outcome.At, slot.coords)

	if err != nil {
		return State{}, err
	}

	if err := inject(handle, solver.config, outcome, oscillators); err != nil {
		return State{}, err
	}

	slot.coords = coordEpoch

	reading, err := handle.Step()

	if err != nil {
		return State{}, err
	}

	slot.epoch++
	slot.at = outcome.At
	slot.events = outcome.EventCount
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
		Duration:         outcome.Horizon,
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

func (solver *Solver) admit(symbol string) (*symbolSlot, error) {
	if symbol == "" {
		return nil, errnie.Err(
			errnie.Internal,
			"manifold: symbol is required",
			nil,
		)
	}

	slot, ok := solver.symbols[symbol]

	if ok {
		return slot, nil
	}

	if len(solver.symbols) >= solver.capacity {
		return nil, errnie.Err(
			errnie.Internal,
			"manifold: symbol capacity exhausted",
			nil,
		)
	}

	var err error

	err = compute.WithMetalInit(func() error {
		solver.handle, err = pmanifold.NewSolver(solver.config)

		if err != nil {
			err = errors.Join(err, errnie.Error(err))
			return err
		}

		slot = &symbolSlot{handle: solver.handle}
		solver.symbols[symbol] = slot

		return nil
	})

	if err != nil {
		err = errors.Join(err, errnie.Error(err))

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
) ([]pmanifold.Oscillator, *coordinateEpoch, error) {
	if solver.books == nil {
		return nil, prior, errnie.Err(
			errnie.Internal,
			"manifold: L3 book source is not configured",
			nil,
		)
	}

	symbolBook, midPrice, ok := bookForSymbol(solver.books, symbol)

	if !ok || symbolBook == nil {
		return nil, prior, errnie.Err(
			errnie.Internal,
			"manifold: authoritative L3 book is unavailable",
			nil,
		)
	}

	orders := ordersFromBook(symbolBook)
	mapped, coordEpoch, ready := mapOrders(solver.config, orders, midPrice, at, prior)

	if !ready {
		return nil, prior, errnie.Err(
			errnie.Internal,
			"manifold: coordinate mapping failed",
			nil,
		)
	}

	oscillators := cohortsFromMappedOrders(solver.config, mapped)

	if len(oscillators) == 0 {
		return nil, prior, errnie.Err(
			errnie.Internal,
			"manifold: cohort extraction produced no carriers",
			nil,
		)
	}

	return oscillators, coordEpoch, nil
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
