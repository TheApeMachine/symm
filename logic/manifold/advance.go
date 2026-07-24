package manifold

import (
	"fmt"
	"math"
	"sort"

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
advance appends Sensorium-shaped particles for every changed Hawkes/book epoch,
then performs at most one GPU step over the full resident history. Unchanged
epochs replay shared readings without duplicating particles. Historical mass
is retained — rule-shifts land in the wave field without culling the tape.
*/
func (solver *Solver) advance(
	thesis *types.Thesis,
	candidates []intensityCandidate,
	changed map[string]excitation.Outcome,
) advanceResult {
	result := advanceResult{}

	if len(candidates) == 0 {
		return result
	}

	failures, grew := solver.appendBatches(candidates, changed)
	result.failures = append(result.failures, failures...)

	if len(result.failures) > 0 {
		return result
	}

	shouldStep := grew && solver.domain.ParticleCount() > 0
	reading, diagnostics, err := solver.step(shouldStep)

	if err != nil {
		result.failures = append(result.failures, err)
		return result
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
Only changed epochs are re-sampled; every other GasReady slot still receives
the new shared reading without a fresh L3 tokenize.
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

	particles := solver.renderPopulation(projection)
	rho := projectionRows(projection.Density, projection.Grid)
	psi := projectionRows(projection.Coherence, projection.Grid)
	guideX := projectionRows(projection.GuidanceX, projection.Grid)
	guideZ := projectionRows(projection.GuidanceZ, projection.Grid)
	sampled := make(map[string]intensityCandidate, len(candidates))

	for _, candidate := range candidates {
		sampled[candidate.symbol] = candidate
	}

	symbols := make([]string, 0, len(solver.symbols))

	for symbol := range solver.symbols {
		symbols = append(symbols, symbol)
	}

	sort.Strings(symbols)

	for _, symbol := range symbols {
		slot := solver.symbols[symbol]

		if slot == nil {
			continue
		}

		outcome, advanced := changed[symbol]
		candidate, hasSample := sampled[symbol]

		if advanced && !hasSample {
			continue
		}

		if !advanced && !slot.last.GasReady() {
			continue
		}

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
		} else {
			result.replayed++
		}

		if err == nil && state.GasReady() {
			at := outcome.At

			if at.IsZero() {
				at = candidate.outcome.At
			}

			if at.IsZero() {
				at = state.At
			}

			phaseScan := state.PhaseScan

			if advanced {
				var phaseErr error
				phaseScan, phaseErr = solver.phase(symbol, at, true, wave)

				if phaseErr != nil {
					result.failures = append(result.failures, phaseErr)
				}
			}

			solver.paint(
				&state, projection.Grid, wave, phaseScan, particles,
				rho, psi, guideX, guideZ,
			)
			slot.last = state
		}

		if state.GasReady() {
			thesis.Manifold.Store(symbol, state)
		}
	}
}

/*
renderPopulation reads the shared resident particles once per advance publish.
*/
func (solver *Solver) renderPopulation(projection pfluid.Projection) []Particle {
	if solver.domain == nil {
		return nil
	}

	population := solver.domain.ParticleCount()

	if population == 0 || len(projection.Density) == 0 {
		return nil
	}

	batch, err := solver.domain.ReadParticles(0, population)

	if err != nil {
		return nil
	}

	spatial, _ := solver.domain.ReadSpatialIDs(0, population)
	return renderParticles(batch, spatial, projection.Grid)
}

/*
appendBatches tokenizes each changed book sample and Appends those particles
into the Metal-resident domain history (Sensorium merge into manifold state).
*/
func (solver *Solver) appendBatches(
	candidates []intensityCandidate,
	changed map[string]excitation.Outcome,
) ([]error, bool) {
	failures := make([]error, 0)
	grew := false

	for _, candidate := range candidates {
		if _, advanced := changed[candidate.symbol]; !advanced {
			continue
		}

		slot := solver.symbols[candidate.symbol].clone()
		batch, err := slot.ingest(candidate)

		if err != nil {
			failures = append(failures, solver.noteAdvanceFailure(candidate.symbol, err))
			continue
		}

		if len(batch.Particles) == 0 {
			failures = append(failures, solver.noteAdvanceFailure(
				candidate.symbol,
				fmt.Errorf("book sample produced no particles"),
			))
			continue
		}

		start, err := solver.domain.Append(batch.Particles, batch.ContentIDs)

		if err != nil {
			failures = append(failures, solver.noteAdvanceFailure(candidate.symbol, err))
			continue
		}

		slot.start = start
		slot.end = start + len(batch.Particles)
		solver.symbols[candidate.symbol] = slot
		solver.active[candidate.symbol] = struct{}{}
		grew = true
	}

	return failures, grew
}

/*
clone creates a transactional symbol slot before an append commits.
*/
func (slot *symbolSlot) clone() *symbolSlot {
	if slot == nil {
		return &symbolSlot{state: make(map[string]pfluid.Particle)}
	}

	cloned := *slot
	return &cloned
}

/*
ingest accepts one owned book sample and bumps the source counter for change
detection. Particles were tokenized under the book lease.
*/
func (slot *symbolSlot) ingest(candidate intensityCandidate) (Batch, error) {
	if len(candidate.batch.Particles) == 0 {
		return Batch{}, fmt.Errorf("L3 sample has no tokenizable orders")
	}

	if candidate.reference == nil || candidate.buyCapacity == nil ||
		candidate.sellCapacity == nil || candidate.spread <= 0 {
		return Batch{}, fmt.Errorf("L3 population has no executable touch")
	}

	slot.orders = append([]string(nil), candidate.orderIDs...)
	slot.at = candidate.outcome.At
	slot.events = candidate.outcome.EventCount
	slot.epoch++

	return candidate.batch, nil
}

/*
step advances the resident Metal population when new observations were
appended, then reads the shared physical reductions. No host re-upload.
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
		if solver.domain.ParticleCount() == 0 {
			return pfluid.Reading{}, pfluid.Diagnostics{}, nil
		}

		reading, err := solver.domain.Reading()
		return reading, pfluid.Diagnostics{}, err
	}

	diagnostics, err := solver.domain.Advance()

	if err != nil {
		return pfluid.Reading{}, diagnostics, errnie.Err(
			errnie.Internal,
			"manifold: shared Sensorium advance failed",
			err,
		)
	}

	// Inelastic merge rewrites resident indices; per-sample ranges are gone.
	solver.clearRanges()

	reading, err := solver.domain.Reading()

	if err != nil {
		return pfluid.Reading{}, diagnostics, err
	}

	return reading, diagnostics, nil
}

/*
clearRanges drops per-symbol particle index ranges after a merge-compacting
Advance. History remains in the shared Metal population; only the append-time
index bookmarks become invalid.
*/
func (solver *Solver) clearRanges() {
	for _, slot := range solver.symbols {
		if slot == nil {
			continue
		}

		slot.start = 0
		slot.end = 0
	}
}

/*
changed reports whether a source sample has not yet been appended into the
resident history.
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
