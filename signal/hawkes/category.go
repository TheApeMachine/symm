package hawkes

import (
	"github.com/theapemachine/symm/logic"
)

const (
	hawkesSaturationRadius = 0.85
	hawkesFrenzyAsymmetry  = 0.15
)

const uniformHawkesConfidence = 1.0 / 4.0

func classifyHawkes(
	fit BivariateFit,
	asymmetry float64,
	sellSide bool,
) (logic.CategoryType, float64, float64, float64, float64, float64) {
	intensity, baseline := fit.BuyIntensity, fit.MuBuy

	if sellSide {
		intensity, baseline = fit.SellIntensity, fit.MuSell
	}

	switch {
	case fit.SpectralRadius >= hawkesSaturationRadius:
		margin := fit.SpectralRadius - hawkesSaturationRadius
		span := 1 - hawkesSaturationRadius

		if margin <= 0 || span <= 0 {
			return logic.CategorySaturation, uniformHawkesConfidence, 0, uniformHawkesConfidence, 0, 0
		}

		score := competitionMargin(margin, span)

		return logic.CategorySaturation, score, 0, score, 0, 0
	case baseline > 0 && intensity < baseline:
		margin := baseline - intensity

		if margin <= 0 {
			return logic.CategoryExhaustion, uniformHawkesConfidence, 0, 0, uniformHawkesConfidence, 0
		}

		score := competitionMargin(margin, baseline)

		return logic.CategoryExhaustion, score, 0, 0, 0, score
	case asymmetry >= hawkesFrenzyAsymmetry:
		margin := asymmetry - hawkesFrenzyAsymmetry
		span := 1 - hawkesFrenzyAsymmetry

		if margin <= 0 || span <= 0 {
			return logic.CategoryFrenzy, uniformHawkesConfidence, uniformHawkesConfidence, 0, 0, 0
		}

		score := competitionMargin(margin, span)

		return logic.CategoryFrenzy, score, score, 0, 0, 0
	default:
		headroom := -1.0
		saturationHead := 0.0
		organicHead := 0.0
		frenzyHead := 0.0

		if fit.SpectralRadius < hawkesSaturationRadius {
			margin := hawkesSaturationRadius - fit.SpectralRadius
			saturationHead = competitionMargin(margin, hawkesSaturationRadius)

			if saturationHead > headroom {
				headroom = saturationHead
			}
		}

		if baseline > 0 && intensity >= baseline {
			margin := intensity - baseline
			organicHead = margin / (margin + baseline)

			if organicHead > headroom {
				headroom = organicHead
			}
		}

		if asymmetry < hawkesFrenzyAsymmetry {
			margin := hawkesFrenzyAsymmetry - asymmetry
			frenzyHead = competitionMargin(margin, hawkesFrenzyAsymmetry)

			if frenzyHead > headroom {
				headroom = frenzyHead
			}
		}

		if headroom < 0 {
			return logic.CategoryOrganic, uniformHawkesConfidence, 0, 0, uniformHawkesConfidence, 0
		}

		return logic.CategoryOrganic, headroom, frenzyHead, saturationHead, organicHead, 0
	}
}

func competitionMargin(excess, span float64) float64 {
	if excess <= 0 || span <= 0 {
		return 0
	}

	return excess / (excess + span)
}
