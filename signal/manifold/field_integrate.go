package manifold

import (
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	mkernel "github.com/theapemachine/nomagique/physics/manifold"
)

func (field *Field) integrate(at time.Time) (bool, error) {
	if ensureErr := field.ensureSolver(); ensureErr != nil {
		return false, ensureErr
	}

	if err := field.solver.ResetDeposits(); err != nil {
		return false, errnie.Error(err)
	}

	type spotCandidate struct {
		state      *UniverseState
		oscillator mkernel.Oscillator
		carrier    fieldCarrier
	}

	candidates := make([]spotCandidate, 0)

	field.universe.states.Range(func(_, value any) bool {
		state, ok := value.(*UniverseState)

		if !ok || !state.bookReady || state.midPrice <= 0 || state.tickSize <= 0 {
			return true
		}

		if state.lane != InstrumentLaneSpot {
			return true
		}

		oscillator := field.oscillatorFromState(state)

		if !oscillatorFullyFinite(oscillator) {
			return true
		}

		candidates = append(candidates, spotCandidate{
			state:      state,
			oscillator: oscillator,
			carrier: fieldCarrier{
				role:       "symbol",
				symbol:     state.symbol,
				oscillator: oscillator,
			},
		})

		return true
	})

	oscillators := make([]mkernel.Oscillator, len(candidates))
	carriers := make([]fieldCarrier, len(candidates))

	for index, candidate := range candidates {
		oscillators[index] = oscillatorForSolver(candidate.oscillator, field.config)
		carriers[index] = candidate.carrier
	}

	if len(oscillators) == 0 && len(field.activeWhales) == 0 && len(field.pendingWhales) == 0 {
		return false, nil
	}

	field.activeWhales = append(field.activeWhales, field.pendingWhales...)
	field.pendingWhales = field.pendingWhales[:0]

	whaleOscillators := make([]mkernel.Oscillator, 0, len(field.activeWhales))
	whaleCarriers := make([]fieldCarrier, 0, len(field.activeWhales))

	for _, whale := range field.activeWhales {
		whaleOscillators = append(whaleOscillators, oscillatorForSolver(whale.oscillator, field.config))
		whaleCarriers = append(whaleCarriers, fieldCarrier{
			role:       "whale",
			symbol:     whale.symbol,
			oscillator: whale.oscillator,
		})
	}

	solverOscillators, solverCarriers := capSolverCarriers(
		oscillators,
		carriers,
		whaleOscillators,
		whaleCarriers,
		field.config.MaxModes,
	)

	if len(solverOscillators) == 0 {
		return false, nil
	}

	carrierCount := len(solverOscillators)

	if field.lastIntegratedCarriers > 0 && field.lastIntegratedCarriers != carrierCount {
		if recreateErr := field.recreateSolver(); recreateErr != nil {
			return false, errnie.Error(recreateErr)
		}
	}

	activeCarriers := carrierCount

	for _, deposit := range field.pendingDeposits {
		if depositErr := field.solver.DepositCell(
			deposit.cellX,
			deposit.cellY,
			deposit.cellZ,
			deposit.rho,
			deposit.momX,
			deposit.momY,
			deposit.momZ,
			deposit.eInt,
		); depositErr != nil {
			return false, errnie.Error(depositErr)
		}
	}

	field.pendingDeposits = field.pendingDeposits[:0]

	activeSymbols := make(map[string]struct{}, len(solverCarriers))

	for _, carrier := range solverCarriers {
		if carrier.role != "symbol" {
			continue
		}

		activeSymbols[carrier.symbol] = struct{}{}
	}

	var depositErr error

	field.universe.states.Range(func(_, value any) bool {
		state, ok := value.(*UniverseState)

		if !ok || !state.bookReady || state.midPrice <= 0 || state.tickSize <= 0 {
			return true
		}

		if state.lane != InstrumentLaneSpot {
			return true
		}

		if _, active := activeSymbols[state.symbol]; !active {
			return true
		}

		if stepErr := field.depositBook(state, activeCarriers); stepErr != nil {
			depositErr = stepErr

			return false
		}

		return true
	})

	if depositErr != nil {
		return false, errnie.Error(depositErr)
	}

	if err := field.solver.SetOscillators(
		normalizeOscillatorsForSolver(
			solverOscillators,
			field.config.RhoMin,
			field.config.MaxModes,
		),
	); err != nil {
		return false, errnie.Error(err)
	}

	reading, err := field.solver.Step()

	if err != nil {
		return false, errnie.Error(err)
	}

	if !readingFinite(reading) {
		return false, fmt.Errorf("manifold: solver reading is non-finite")
	}

	readOscillators, err := field.solver.ReadOscillators(len(solverOscillators))

	if err != nil {
		return false, errnie.Error(err)
	}

	field.activeWhales = field.whalesFromSolverReadback(solverCarriers, readOscillators)
	field.lastReading = reading
	field.lastStepAt = at
	field.lastIntegratedCarriers = carrierCount
	field.lastCarriers = field.displayCarriers(carriers, solverCarriers, readOscillators)

	field.universe.states.Range(func(_, value any) bool {
		state, ok := value.(*UniverseState)

		if !ok || state.lane != InstrumentLaneSpot {
			return true
		}

		field.readings.Delete(state.symbol)

		return true
	})

	for _, carrier := range solverCarriers {
		if carrier.role != "symbol" {
			continue
		}

		state := field.universe.loadSymbol(carrier.symbol)
		price := 0.0

		if state != nil {
			price = state.lastPrice
		}

		field.readings.Store(carrier.symbol, symbolReading{
			reading: reading,
			price:   price,
			at:      at,
		})
	}

	return true, nil
}
