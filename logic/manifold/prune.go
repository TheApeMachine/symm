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
retainAboveMedian returns indices whose contribution is at least the median of
positive contributors. Zero-mass and zero-store particles are always dropped.
Median (not mean) keeps the typical half of a skewed Hawkes intensity mix —
mean culling collapses a multi-symbol domain onto the single hottest oscillator
and blanks the pilot-wave projection.
*/
func retainAboveMedian(particles []pfluid.Particle) []uint32 {
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
	threshold := positives[(len(positives)-1)/2]
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
pruneInert drops resident particles whose mass×(energy+heat) sits below the
positive-population median after an append Advance. Merge already compacted
same-cell twins; this removes the long tail of negligible contributors so GPU
and wire cost cannot grow without bound from historical dust. Callers must not
invoke this on pure always-step ticks.
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

	kept := retainAboveMedian(particles)

	// Empty keep-set means no positive contributors — leave the resident
	// population alone rather than Retain(nil) wiping the domain.
	if len(kept) == 0 || len(kept) >= population {
		return nil
	}

	return solver.domain.Retain(kept)
}
