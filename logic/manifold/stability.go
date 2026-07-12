package manifold

import (
	"math"

	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

const stabilitySafety = 0.99

/*
EventSubdivisions returns how many solver substeps are required for eventDeltaT
to satisfy advective and diffusive CFL limits implied by cohort motion.
*/
func EventSubdivisions(
	config *pmanifold.Config,
	eventDeltaT float64,
	cohorts []Cohort,
) int {
	if config == nil || eventDeltaT <= 0 {
		return 0
	}

	limits := []float64{eventDeltaT}

	dx := config.DomainX / float64(config.GridX)
	dy := config.DomainY / float64(config.GridY)
	dz := config.DomainZ / float64(config.GridZ)
	waveSpeed := maxCohortWaveSpeed(config.Gamma, cohorts)

	if advectiveLimit := config.AdvectiveDeltaT(waveSpeed); advectiveLimit > 0 {
		limits = append(limits, advectiveLimit*stabilitySafety)
	}

	envelopeRho := config.GasEnvelopeRhoMin

	if envelopeRho > 0 && config.CV > 0 && config.KThermal > 0 {
		inverseSpacingSum := 0.0

		if dx > 0 {
			inverseSpacingSum += 1.0 / (dx * dx)
		}

		if dy > 0 {
			inverseSpacingSum += 1.0 / (dy * dy)
		}

		if dz > 0 {
			inverseSpacingSum += 1.0 / (dz * dz)
		}

		if inverseSpacingSum > 0 {
			diffusionLimit := envelopeRho * config.CV * 0.5 /
				(config.KThermal * inverseSpacingSum) * stabilitySafety
			limits = append(limits, diffusionLimit)
		}
	}

	substep := limits[0]

	for _, limit := range limits[1:] {
		if limit > 0 && limit < substep {
			substep = limit
		}
	}

	if substep <= 0 {
		return 0
	}

	subdivisions := int(math.Ceil(eventDeltaT / substep))

	if subdivisions < 1 {
		return 1
	}

	return subdivisions
}

func maxCohortWaveSpeed(gamma float64, cohorts []Cohort) float64 {
	waveSpeed := 0.0

	for _, cohort := range cohorts {
		velocity := math.Hypot(
			cohort.Velocity.Price,
			math.Hypot(cohort.Velocity.Size, cohort.Velocity.Age),
		)
		trace := cohort.SecondMoment.Price +
			cohort.SecondMoment.Size +
			cohort.SecondMoment.Age
		soundSpeed := math.Sqrt(max(gamma*(gamma-1)*trace/2, 0))
		rarefactionSpeed := velocity + 2*soundSpeed/(gamma-1)
		waveSpeed = max(waveSpeed, rarefactionSpeed)
	}

	return waveSpeed
}
