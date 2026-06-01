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
categoryConfidence returns how decisively the fluid category wins over its
neighbors — not how large the Reynolds number is.
*/
func categoryConfidence(
	category perspectives.CategoryType,
	divergence, turbulence, viscosity, reynolds float64,
) float64 {
	absDivergence := math.Abs(divergence)

	switch category {
	case perspectives.CategoryTurbulent:
		return turbulentConfidence(turbulence, absDivergence)
	case perspectives.CategoryViscous:
		return viscousConfidence(viscosity)
	case perspectives.CategoryInertial:
		return inertialConfidence(absDivergence, reynolds)
	case perspectives.CategoryLaminar:
		return laminarConfidence(absDivergence, turbulence, viscosity, reynolds)
	default:
		return 0
	}
}

func turbulentConfidence(turbulence, absDivergence float64) float64 {
	margin := turbulence - absDivergence

	if margin <= 0 {
		return 0
	}

	scale := math.Max(turbulence, absDivergence)

	if scale <= 0 {
		return 0
	}

	return margin / scale
}

func viscousConfidence(viscosity float64) float64 {
	margin := fluidViscousThreshold - viscosity

	if margin <= 0 {
		return 0
	}

	return margin / fluidViscousThreshold
}

func inertialConfidence(absDivergence, reynolds float64) float64 {
	divMargin := absDivergence - fluidInertialThreshold
	reMargin := reynolds - fluidInertialThreshold
	margin := math.Min(divMargin, reMargin)

	if margin <= 0 {
		return 0
	}

	return margin / fluidInertialThreshold
}

func laminarConfidence(absDivergence, turbulence, viscosity, reynolds float64) float64 {
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
		return 0
	}

	return math.Min(1, headroom/fluidInertialThreshold)
}
