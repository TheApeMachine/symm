package hawkes

import (
	"github.com/theapemachine/symm/market/perspectives/types"
)

const (
	hawkesSaturationRadius = 0.85
	hawkesFrenzyAsymmetry  = 0.15
)

/*
hawkesReading classifies the fitted Hawkes state and returns shift evidence.
*/
func hawkesReading(fit BivariateFit, asymmetry float64, sellSide bool) (types.CategoryType, float64) {
	intensity, baseline := fit.BuyIntensity, fit.MuBuy

	if sellSide {
		intensity, baseline = fit.SellIntensity, fit.MuSell
	}

	switch {
	case fit.SpectralRadius >= hawkesSaturationRadius:
		margin := fit.SpectralRadius - hawkesSaturationRadius
		span := 1 - hawkesSaturationRadius

		if margin <= 0 || span <= 0 {
			return types.CategorySaturation, 0
		}

		return types.CategorySaturation, margin / span
	case baseline > 0 && intensity < baseline:
		margin := baseline - intensity

		if margin <= 0 {
			return types.CategoryExhaustion, 0
		}

		return types.CategoryExhaustion, margin / baseline
	case asymmetry >= hawkesFrenzyAsymmetry:
		margin := asymmetry - hawkesFrenzyAsymmetry
		span := 1 - hawkesFrenzyAsymmetry

		if margin <= 0 || span <= 0 {
			return types.CategoryFrenzy, 0
		}

		return types.CategoryFrenzy, margin / span
	default:
		headroom := -1.0

		if fit.SpectralRadius < hawkesSaturationRadius {
			margin := hawkesSaturationRadius - fit.SpectralRadius

			if margin/hawkesSaturationRadius > headroom {
				headroom = margin / hawkesSaturationRadius
			}
		}

		if baseline > 0 && intensity >= baseline {
			margin := intensity - baseline
			score := margin / (margin + baseline)

			if score > headroom {
				headroom = score
			}
		}

		if asymmetry < hawkesFrenzyAsymmetry {
			margin := (hawkesFrenzyAsymmetry - asymmetry) / hawkesFrenzyAsymmetry

			if margin > headroom {
				headroom = margin
			}
		}

		if headroom < 0 {
			return types.CategoryOrganic, 0
		}

		return types.CategoryOrganic, headroom
	}
}
