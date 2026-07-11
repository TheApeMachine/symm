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
	velocity := maxCohortVelocity(cohorts)

	dx := config.DomainX / float64(config.GridX)
	dy := config.DomainY / float64(config.GridY)
	dz := config.DomainZ / float64(config.GridZ)

	if velocity.Price > 0 && dx > 0 {
		limits = append(limits, dx/velocity.Price*stabilitySafety)
	}

	if velocity.Size > 0 && dy > 0 {
		limits = append(limits, dy/velocity.Size*stabilitySafety)
	}

	if velocity.Age > 0 && dz > 0 {
		limits = append(limits, dz/velocity.Age*stabilitySafety)
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

func maxCohortVelocity(cohorts []Cohort) Coordinate {
	velocity := Coordinate{}

	for _, cohort := range cohorts {
		velocity.Price = math.Max(velocity.Price, math.Abs(cohort.Velocity.Price))
		velocity.Size = math.Max(velocity.Size, math.Abs(cohort.Velocity.Size))
		velocity.Age = math.Max(velocity.Age, math.Abs(cohort.Velocity.Age))
	}

	return velocity
}
