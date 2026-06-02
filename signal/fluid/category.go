package fluid

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives"
)

const (
	fluidInertialThreshold = 0.2
	fluidViscousThreshold  = 0.5
)

/*
fluidReading classifies the fluid field and returns shift evidence — margin to
the nearest competing category boundary.
*/
func fluidReading(
	divergence, turbulence, viscosity, reynolds float64,
) (perspectives.CategoryType, float64) {
	absDivergence := math.Abs(divergence)

	switch {
	case turbulence > 0 && turbulence >= absDivergence:
		margin := turbulence - absDivergence

		if margin <= 0 {
			return perspectives.CategoryTurbulent, 0
		}

		scale := math.Max(turbulence, absDivergence)

		if scale <= 0 {
			return perspectives.CategoryTurbulent, 0
		}

		return perspectives.CategoryTurbulent, margin / scale
	case viscosity > 0 && viscosity < fluidViscousThreshold:
		margin := fluidViscousThreshold - viscosity

		if margin <= 0 {
			return perspectives.CategoryViscous, 0
		}

		return perspectives.CategoryViscous, margin / fluidViscousThreshold
	case absDivergence >= fluidInertialThreshold && reynolds >= fluidInertialThreshold:
		margin := math.Min(
			absDivergence-fluidInertialThreshold,
			reynolds-fluidInertialThreshold,
		)

		if margin <= 0 {
			return perspectives.CategoryInertial, 0
		}

		return perspectives.CategoryInertial, margin / fluidInertialThreshold
	default:
		headroom := math.MaxFloat64

		if turbulence > 0 {
			turbulentMargin := absDivergence - turbulence

			if turbulentMargin < headroom {
				headroom = turbulentMargin
			}
		}

		if viscosity > 0 && viscosity < fluidViscousThreshold {
			viscousMargin := fluidViscousThreshold - viscosity

			if viscousMargin < headroom {
				headroom = viscousMargin
			}
		}

		if absDivergence >= fluidInertialThreshold && reynolds >= fluidInertialThreshold {
			inertialMargin := math.Min(
				absDivergence-fluidInertialThreshold,
				reynolds-fluidInertialThreshold,
			)

			if inertialMargin < headroom {
				headroom = inertialMargin
			}
		}

		if headroom == math.MaxFloat64 || headroom <= 0 {
			return perspectives.CategoryLaminar, 0
		}

		return perspectives.CategoryLaminar, headroom / (headroom + fluidInertialThreshold)
	}
}
