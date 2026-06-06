package hawkes

import (
	"github.com/theapemachine/symm/market/perspectives/types"
)

const (
	hawkesSaturationRadius = 0.85
	hawkesFrenzyAsymmetry  = 0.15
)

// uniformHawkesConfidence is the 1/N floor across the four hawkes categories
// (saturation, exhaustion, frenzy, organic): a degenerate read with no margin is a
// uniform guess, never 0. This fallback path runs only when no classifier is wired.
const uniformHawkesConfidence = 1.0 / 4.0

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
			return types.CategorySaturation, uniformHawkesConfidence
		}

		return types.CategorySaturation, types.UnitCompetitionMargin(margin, span)
	case baseline > 0 && intensity < baseline:
		margin := baseline - intensity

		if margin <= 0 {
			return types.CategoryExhaustion, uniformHawkesConfidence
		}

		return types.CategoryExhaustion, types.UnitCompetitionMargin(margin, baseline)
	case asymmetry >= hawkesFrenzyAsymmetry:
		margin := asymmetry - hawkesFrenzyAsymmetry
		span := 1 - hawkesFrenzyAsymmetry

		if margin <= 0 || span <= 0 {
			return types.CategoryFrenzy, uniformHawkesConfidence
		}

		return types.CategoryFrenzy, types.UnitCompetitionMargin(margin, span)
	default:
		headroom := -1.0

		if fit.SpectralRadius < hawkesSaturationRadius {
			margin := hawkesSaturationRadius - fit.SpectralRadius

			score := types.UnitCompetitionMargin(margin, hawkesSaturationRadius)

			if score > headroom {
				headroom = score
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
			margin := hawkesFrenzyAsymmetry - asymmetry
			score := types.UnitCompetitionMargin(margin, hawkesFrenzyAsymmetry)

			if score > headroom {
				headroom = score
			}
		}

		if headroom < 0 {
			return types.CategoryOrganic, uniformHawkesConfidence
		}

		return types.CategoryOrganic, headroom
	}
}
