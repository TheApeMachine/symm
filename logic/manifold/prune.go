package manifold

import (
	"sort"

	pfluid "github.com/theapemachine/nomagique/physics/fluid"
)

/*
particleContribution is the load-bearing scalar for retention: mass times the
sum of oscillator energy and thermal heat. Heat alone is not death, and energy
alone ignores gas scatter mass.
*/
func particleContribution(particle pfluid.Particle) float64 {
	if particle.Mass <= 0 {
		return 0
	}

	store := float64(particle.Energy) + float64(particle.Heat)

	if store < 0 {
		store = 0
	}

	return float64(particle.Mass) * store
}

/*
retainAboveDustTail returns indices whose contribution clears the lower quantile
of positive contributors. Zero-mass and zero-store particles are always
dropped. The shared manifold can carry a large resident history, so pruning
should only remove the inert dust tail rather than halve the population on each
growth step.
*/
func retainAboveDustTail(particles []pfluid.Particle) []uint32 {
	const keepFloor = 0.80

	if len(particles) == 0 {
		return nil
	}

	scores := make([]float64, len(particles))
	positives := make([]float64, 0, len(particles))

	for index, particle := range particles {
		score := particleContribution(particle)
		scores[index] = score

		if score <= 0 {
			continue
		}

		positives = append(positives, score)
	}

	if len(positives) == 0 {
		return nil
	}

	sort.Float64s(positives)
	thresholdIndex := int(float64(len(positives)-1) * (1.0 - keepFloor))
	threshold := positives[thresholdIndex]
	kept := make([]uint32, 0, len(positives))

	for index, score := range scores {
		if score >= threshold {
			kept = append(kept, uint32(index))
		}
	}

	if len(kept) == 0 {
		best := 0

		for index, score := range scores {
			if score > scores[best] {
				best = index
			}
		}

		return []uint32{uint32(best)}
	}

	sort.Slice(kept, func(left, right int) bool {
		return kept[left] < kept[right]
	})

	return kept
}

/*
pruneInert drops resident particles whose mass×(energy+heat) sits in the lower
dust tail of the positive population after an append Advance. Merge already
compacted same-cell twins; this trims negligible contributors while keeping the
large majority of resident history alive. Callers must not invoke this on pure
always-step ticks.
*/
func (solver *Solver) pruneInert() error {
	if solver == nil || solver.domain == nil {
		return nil
	}

	population := solver.domain.ParticleCount()

	if population < 2 {
		return nil
	}

	particles, err := solver.domain.ReadParticles(0, population)

	if err != nil {
		return err
	}

	kept := retainAboveDustTail(particles)

	// Empty keep-set means no positive contributors — leave the resident
	// population alone rather than Retain(nil) wiping the domain.
	if len(kept) == 0 || len(kept) >= population {
		return nil
	}

	return solver.domain.Retain(kept)
}
