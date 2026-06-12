package hawkes

import (
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/logic"
	hkernel "github.com/theapemachine/nomagique/kernel/hawkes"
)

const (
	hawkesSaturationRadius = 0.85
	hawkesFrenzyAsymmetry  = 0.15
)

const uniformHawkesConfidence = 1.0 / 4.0

func classifyHawkes(
	fit hkernel.BivariateFit,
	asymmetry float64,
	sellSide bool,
) (logic.CategoryType, float64, float64, float64, float64, float64) {
	category, confidence := hkernel.ClassifyFit(fit, asymmetry, sellSide)
	logicCategory := fitCategoryToLogic(category)

	frenzy, saturation, organic, exhaustion := transitionScores(
		fit, asymmetry, sellSide, category, confidence,
	)

	return logicCategory, confidence, frenzy, saturation, organic, exhaustion
}

func fitCategoryToLogic(category hkernel.FitCategory) logic.CategoryType {
	switch category {
	case hkernel.FitCategoryFrenzy:
		return logic.CategoryFrenzy
	case hkernel.FitCategorySaturation:
		return logic.CategorySaturation
	case hkernel.FitCategoryExhaustion:
		return logic.CategoryExhaustion
	default:
		return logic.CategoryOrganic
	}
}

func transitionScores(
	fit hkernel.BivariateFit,
	asymmetry float64,
	sellSide bool,
	category hkernel.FitCategory,
	confidence float64,
) (frenzy, saturation, organic, exhaustion float64) {
	switch category {
	case hkernel.FitCategorySaturation:
		return 0, confidence, 0, 0
	case hkernel.FitCategoryExhaustion:
		return 0, 0, 0, confidence
	case hkernel.FitCategoryFrenzy:
		return confidence, 0, 0, 0
	default:
		return organicHeadroomScores(fit, asymmetry, sellSide)
	}
}

func organicHeadroomScores(
	fit hkernel.BivariateFit,
	asymmetry float64,
	sellSide bool,
) (frenzy, saturation, organic, exhaustion float64) {
	intensity, baseline := fit.IntensityX, fit.MuX

	if sellSide {
		intensity, baseline = fit.IntensityY, fit.MuY
	}

	headroom := -1.0

	if fit.SpectralRadius < hawkesSaturationRadius {
		margin := hawkesSaturationRadius - fit.SpectralRadius
		saturation = probability.CompetitionMargin(margin, hawkesSaturationRadius)

		if saturation > headroom {
			headroom = saturation
		}
	}

	if baseline > 0 && intensity >= baseline {
		margin := intensity - baseline
		organic = margin / (margin + baseline)

		if organic > headroom {
			headroom = organic
		}
	}

	if asymmetry < hawkesFrenzyAsymmetry {
		margin := hawkesFrenzyAsymmetry - asymmetry
		frenzy = probability.CompetitionMargin(margin, hawkesFrenzyAsymmetry)

		if frenzy > headroom {
			headroom = frenzy
		}
	}

	if headroom < 0 {
		headroom = uniformHawkesConfidence
	}

	return frenzy, saturation, organic, exhaustion
}

func competitionMargin(excess, span float64) float64 {
	return probability.CompetitionMargin(excess, span)
}
