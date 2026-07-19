package manifold

import (
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/signal/compute"
)

/*
advance steps one symbol's GPU field from a Hawkes outcome, or republishes the
last GasReady projection when the excitation epoch has not moved. focus is the
cut-wide ui.manifold_focus resolved once by Update and threaded through
advanceAll so each symbol does not re-read viper.
*/
func (solver *Solver) advance(
	symbol string,
	outcome excitation.Outcome,
	focus string,
) (State, error) {
	slot := solver.symbols[symbol]

	if slot != nil && (outcome.At.Before(slot.at) ||
		(outcome.At.Equal(slot.at) && outcome.EventCount == slot.events)) {
		if slot.last.GasReady() {
			replay := slot.last
			replay.Replay = true
			return replay, nil
		}

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
	characteristicSpeed, scale, err := applyForcing(
		solver.config,
		outcome,
		interval,
		oscillators,
	)

	if err != nil {
		return State{}, err
	}

	if scale < 1 {
		errnie.Error(audit.Record(solver.recorder, "manifold_force", map[string]any{
			"symbol": symbol,
			"scale":  scale,
			"speed":  characteristicSpeed,
		}))
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

	focused := focus != "" && focus == symbol
	projection, projectionErr := projectField(handle, solver.config, len(oscillators), focused)

	if projectionErr != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("manifold: %s failed to read field projection", symbol),
			projectionErr,
		))
	}

	state := State{
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
	}
	slot.last = state

	return state, nil
}

/*
admit allocates a resident Field on the shared Metal engine for the requested
symbol. Fields stay resident until the active-set budget is exceeded and the
field goes cold, at which point Update evicts it through Solver.release.
*/
func (solver *Solver) admit(symbol string) (*symbolSlot, error) {
	if symbol == "" {
		return nil, errnie.Err(
			errnie.Internal,
			"manifold: symbol is required",
			nil,
		)
	}

	if solver.engine == nil {
		return nil, errnie.Err(
			errnie.Internal,
			"manifold: shared engine is not initialized",
			nil,
		)
	}

	slot, ok := solver.symbols[symbol]

	if ok && slot.handle != nil {
		return slot, nil
	}

	err := compute.WithMetalInit(func() error {
		handle, err := solver.engine.NewField()

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
			"manifold: failed to allocate field",
			err,
		)
	}

	return slot, nil
}

/*
populationOscillators maps the current L3 book into injection cohorts for one
symbol, reusing the prior coordinate epoch when the book has not re-anchored.
*/
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
