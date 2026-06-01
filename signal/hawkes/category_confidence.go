package hawkes

import (
	"github.com/theapemachine/symm/market/perspectives"
)

const (
	hawkesSaturationRadius = 0.85
	hawkesFrenzyAsymmetry  = 0.15
)

/*
categoryConfidence returns how decisively the thermal category wins over its
neighbors — not how large the intensity-over-μ strength is.
*/
func categoryConfidence(
	category perspectives.CategoryType,
	fit BivariateFit,
	asymmetry float64,
	sellSide bool,
) float64 {
	intensity, baseline := fit.BuyIntensity, fit.MuBuy

	if sellSide {
		intensity, baseline = fit.SellIntensity, fit.MuSell
	}

	switch category {
	case perspectives.CategorySaturation:
		return saturationConfidence(fit.SpectralRadius)
	case perspectives.CategoryExhaustion:
		return exhaustionConfidence(intensity, baseline)
	case perspectives.CategoryFrenzy:
		return frenzyConfidence(asymmetry)
	case perspectives.CategoryOrganic:
		return organicConfidence(fit.SpectralRadius, intensity, baseline, asymmetry)
	default:
		return 0
	}
}

func saturationConfidence(spectralRadius float64) float64 {
	margin := spectralRadius - hawkesSaturationRadius

	if margin <= 0 {
		return 0
	}

	span := 1 - hawkesSaturationRadius

	if span <= 0 {
		return 0
	}

	return margin / span
}

func exhaustionConfidence(intensity, baseline float64) float64 {
	if baseline <= 0 {
		return 0
	}

	margin := baseline - intensity

	if margin <= 0 {
		return 0
	}

	return margin / baseline
}

func frenzyConfidence(asymmetry float64) float64 {
	margin := asymmetry - hawkesFrenzyAsymmetry

	if margin <= 0 {
		return 0
	}

	span := 1 - hawkesFrenzyAsymmetry

	if span <= 0 {
		return 0
	}

	return margin / span
}

func organicConfidence(
	spectralRadius, intensity, baseline, asymmetry float64,
) float64 {
	headroom := -1.0

	if spectralRadius < hawkesSaturationRadius {
		margin := hawkesSaturationRadius - spectralRadius

		if margin > headroom {
			headroom = margin / hawkesSaturationRadius
		}
	}

	if baseline > 0 && intensity >= baseline {
		margin := (intensity - baseline) / baseline

		if margin > headroom {
			headroom = margin
		}
	}

	if asymmetry < hawkesFrenzyAsymmetry {
		margin := (hawkesFrenzyAsymmetry - asymmetry) / hawkesFrenzyAsymmetry

		if margin > headroom {
			headroom = margin
		}
	}

	if headroom < 0 {
		return 0
	}

	return headroom
}
