package manifold

import (
	"fmt"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
	"github.com/theapemachine/symm/types"
)

/*
advanceResult describes one shared physical step and every source epoch that
was either represented by it or replayed from it.
*/
type advanceResult struct {
	failures []error
	advanced int
	replayed int
}

/*
advance detects changed source epochs, assembles the complete current universe,
performs at most one GPU step, and publishes views of the resulting shared field.
*/
func (solver *Solver) advance(
	thesis *types.Thesis,
	candidates []intensityCandidate,
) advanceResult {
	result := advanceResult{}
	changed := make(map[string]excitation.Outcome)

	for _, candidate := range candidates {
		if candidate.changed(solver.symbols[candidate.symbol]) {
			changed[candidate.symbol] = candidate.outcome
		}
	}

	populationChanged := solver.universeChanged(candidates)
	shouldStep := len(changed) > 0 || populationChanged

	if shouldStep {
		result.failures = append(result.failures, solver.populate(candidates, changed)...)

		if len(result.failures) > 0 {
			return result
		}
	}

	reading, diagnostics, err := solver.step(shouldStep && len(solver.particles) > 0)

	if err != nil {
		result.failures = append(result.failures, err)
		return result
	}

	if shouldStep && len(solver.particles) > 0 {
		solver.retain(candidates)
	}

	solver.publish(
		thesis,
		candidates,
		changed,
		reading,
		diagnostics,
		&result,
	)
	return result
}

/*
publish materializes symbol views from one shared reading and attaches the
field projection to every GasReady symbol so the UI can select client-side.
*/
func (solver *Solver) publish(
	thesis *types.Thesis,
	candidates []intensityCandidate,
	changed map[string]excitation.Outcome,
	reading pfluid.Reading,
	diagnostics pfluid.Diagnostics,
	result *advanceResult,
) {
	projection, wave, err := solver.project()

	if err != nil {
		result.failures = append(result.failures, err)
	}

	for _, candidate := range candidates {
		slot := solver.symbols[candidate.symbol]

		if slot == nil {
			continue
		}

		outcome, advanced := changed[candidate.symbol]
		state := slot.view(
			candidate,
			outcome,
			reading,
			diagnostics,
			solver.config.Grid,
			advanced,
		)

		if advanced {
			result.advanced++
		} else if state.GasReady() {
			result.replayed++
		}

		if err == nil && state.GasReady() {
			at := outcome.At

			if at.IsZero() {
				at = candidate.outcome.At
			}

			_, phaseScan, phaseErr := solver.phase(candidate.symbol, at, advanced)

			if phaseErr != nil {
				result.failures = append(result.failures, phaseErr)
			}

			solver.paint(&state, projection, wave, phaseScan, slot)
		}

		if state.GasReady() {
			thesis.Manifold.Store(candidate.symbol, state)
		}
	}
}

/*
populate maps the complete current universe and preserves the evolved phase and
energy of surviving order identities before the next shared physical step.
*/
func (solver *Solver) populate(
	candidates []intensityCandidate,
	changed map[string]excitation.Outcome,
) []error {
	population := make([]pfluid.Particle, 0, len(solver.particles))
	failures := make([]error, 0)
	nextSlots := make(map[string]*symbolSlot, len(candidates))

	for _, candidate := range candidates {
		slot := solver.symbols[candidate.symbol].clone()
		particles, err := slot.observe(
			solver.config, candidate, len(population), changed,
		)

		if err != nil {
			failures = append(failures, solver.noteAdvanceFailure(candidate.symbol, err))
			continue
		}

		nextSlots[candidate.symbol] = slot
		population = append(population, particles...)
	}

	if len(failures) > 0 {
		return failures
	}

	solver.particles = population
	solver.active = make(map[string]struct{}, len(candidates))

	for _, candidate := range candidates {
		solver.active[candidate.symbol] = struct{}{}
		solver.symbols[candidate.symbol] = nextSlots[candidate.symbol]
	}

	return failures
}

/*
clone creates a transactional symbol slot whose resident particle map is read
only until the complete universe has mapped successfully.
*/
func (slot *symbolSlot) clone() *symbolSlot {
	if slot == nil {
		return &symbolSlot{state: make(map[string]pfluid.Particle)}
	}

	cloned := *slot
	return &cloned
}

/*
observe maps one symbol into a staged population range and advances its source
epoch only when the Hawkes observation changed.
*/
func (slot *symbolSlot) observe(
	config pfluid.Config,
	candidate intensityCandidate,
	start int,
	changed map[string]excitation.Outcome,
) ([]pfluid.Particle, error) {
	particles, epoch, mapped := slot.coords.Map(config, candidate)

	if !mapped {
		return nil, fmt.Errorf("L3 population has no physical coordinates")
	}

	slot.start = start
	slot.end = start + len(particles)
	slot.orders = make([]string, len(particles))
	slot.coords = epoch

	for index := range particles {
		orderID := candidate.orders[index].orderID
		slot.orders[index] = orderID
		particles[index] = slot.preserve(orderID, particles[index])
	}

	if outcome, advanced := changed[candidate.symbol]; advanced {
		slot.at = outcome.At
		slot.events = outcome.EventCount
		slot.epoch++
	}

	return particles, nil
}

/*
universeChanged reports whether symbol admission or removal changes the
complete population even when every surviving source epoch is unchanged.
*/
func (solver *Solver) universeChanged(candidates []intensityCandidate) bool {
	if len(candidates) != len(solver.active) {
		return true
	}

	for _, candidate := range candidates {
		if _, active := solver.active[candidate.symbol]; !active {
			return true
		}
	}

	return false
}

/*
preserve transfers resident thermodynamic and phase state to the freshly
observed geometry of an order that survived from the preceding population.
*/
func (slot *symbolSlot) preserve(
	orderID string,
	observation pfluid.Particle,
) pfluid.Particle {
	resident, found := slot.state[orderID]

	if !found {
		return observation
	}

	observation.Heat = resident.Heat
	observation.Energy = resident.Energy
	observation.Phase = resident.Phase
	return observation
}

/*
retain indexes the post-step particle state by stable order identity so the next
complete population can remove canceled orders without losing surviving phase.
*/
func (solver *Solver) retain(candidates []intensityCandidate) {
	for _, candidate := range candidates {
		slot := solver.symbols[candidate.symbol]

		if slot == nil || slot.end > len(solver.particles) {
			continue
		}

		slot.state = make(map[string]pfluid.Particle, len(slot.orders))

		for index, orderID := range slot.orders {
			slot.state[orderID] = solver.particles[slot.start+index]
		}
	}
}

/*
step advances the shared domain when new observations exist, then reads the
same physical reductions used by every symbol view.
*/
func (solver *Solver) step(changed bool) (
	pfluid.Reading,
	pfluid.Diagnostics,
	error,
) {
	if solver.domain == nil {
		return pfluid.Reading{}, pfluid.Diagnostics{}, errnie.Err(
			errnie.Internal,
			"manifold: shared Sensorium domain is closed",
			nil,
		)
	}

	if !changed {
		if len(solver.particles) == 0 {
			return pfluid.Reading{}, pfluid.Diagnostics{}, nil
		}

		reading, err := solver.domain.Reading()
		return reading, pfluid.Diagnostics{}, err
	}

	diagnostics, err := solver.domain.Step(solver.particles)

	if err != nil {
		return pfluid.Reading{}, diagnostics, errnie.Err(
			errnie.Internal,
			"manifold: shared Sensorium step failed",
			err,
		)
	}

	reading, err := solver.domain.Reading()

	if err != nil {
		return pfluid.Reading{}, diagnostics, err
	}

	return reading, diagnostics, nil
}

/*
changed reports whether a source epoch has not yet entered the shared domain.
*/
func (candidate intensityCandidate) changed(slot *symbolSlot) bool {
	if slot == nil || !slot.last.GasReady() {
		return true
	}

	return candidate.outcome.At.After(slot.at) ||
		(candidate.outcome.At.Equal(slot.at) && candidate.outcome.EventCount != slot.events)
}

/*
intensities selects fitted Hawkes intensity only when that fit is ready;
otherwise it retains the directly observed arrival rates.
*/
func intensities(outcome excitation.Outcome) (float64, float64) {
	if outcome.Readiness.HawkesFit {
		return outcome.Fit.IntensityX, outcome.Fit.IntensityY
	}

	return outcome.BuyArrivalRate, outcome.SellArrivalRate
}

/*
stressAnisotropy is the dimensionless imbalance between fitted self-excitation
terms and remains zero before those terms exist.
*/
func stressAnisotropy(outcome excitation.Outcome) float64 {
	selfSum := outcome.Fit.AlphaXX + outcome.Fit.AlphaYY

	if selfSum <= 0 {
		return 0
	}

	return math.Abs(outcome.Fit.AlphaXX-outcome.Fit.AlphaYY) / selfSum
}
