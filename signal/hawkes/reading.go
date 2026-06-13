package hawkes

import (
	"math"

	"github.com/theapemachine/nomagique/hawkes"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/logic"
)

const (
	hawkesSaturationRadius = 0.85
	hawkesFrenzyAsymmetry  = 0.15
)

const uniformHawkesConfidence = 1.0 / 4.0

func classifyHawkes(
	fit hawkes.BivariateFit,
	asymmetry float64,
	sellSide bool,
) (logic.CategoryType, float64, float64, float64, float64, float64) {
	category, confidence := hawkes.ClassifyFit(fit, asymmetry, sellSide)
	logicCategory := fitCategoryToLogic(category)

	frenzy, saturation, organic, exhaustion := transitionScores(
		fit, asymmetry, sellSide, category, confidence,
	)

	return logicCategory, confidence, frenzy, saturation, organic, exhaustion
}

func fitCategoryToLogic(category hawkes.FitCategory) logic.CategoryType {
	switch category {
	case hawkes.FitCategoryFrenzy:
		return logic.CategoryFrenzy
	case hawkes.FitCategorySaturation:
		return logic.CategorySaturation
	case hawkes.FitCategoryExhaustion:
		return logic.CategoryExhaustion
	default:
		return logic.CategoryOrganic
	}
}

func transitionScores(
	fit hawkes.BivariateFit,
	asymmetry float64,
	sellSide bool,
	category hawkes.FitCategory,
	confidence float64,
) (frenzy, saturation, organic, exhaustion float64) {
	frenzy, saturation, organic, exhaustion = organicHeadroomScores(fit, asymmetry, sellSide)

	switch category {
	case hawkes.FitCategorySaturation:
		saturation = math.Max(saturation, confidence)
	case hawkes.FitCategoryExhaustion:
		exhaustion = math.Max(exhaustion, confidence)
	case hawkes.FitCategoryFrenzy:
		frenzy = math.Max(frenzy, confidence)
	default:
		organic = math.Max(organic, confidence)
	}

	return frenzy, saturation, organic, exhaustion
}

func organicHeadroomScores(
	fit hawkes.BivariateFit,
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
