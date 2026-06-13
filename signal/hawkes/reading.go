package hawkes

import (
	"math"

	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique/hawkes"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

const uniformHawkesConfidence = 1.0 / 4.0

func classifyHawkes(
	fit hawkes.BivariateFit,
	asymmetry float64,
	sellSide bool,
	gates hawkes.FitGates,
) (logic.CategoryType, float64, float64, float64, float64, float64) {
	gates = temperatureScaledGates(gates)
	category, confidence := hawkes.ClassifyFit(fit, asymmetry, sellSide, gates)
	logicCategory := fitCategoryToLogic(category)

	frenzy, saturation, organic, exhaustion := transitionScores(
		fit, asymmetry, sellSide, category, confidence, gates,
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
	gates hawkes.FitGates,
) (frenzy, saturation, organic, exhaustion float64) {
	frenzy, saturation, organic, exhaustion = organicHeadroomScores(fit, asymmetry, sellSide, gates)

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
	gates hawkes.FitGates,
) (frenzy, saturation, organic, exhaustion float64) {
	if !gates.Ready() {
		return 0, 0, 0, 0
	}

	intensity, baseline := fit.IntensityX, fit.MuX

	if sellSide {
		intensity, baseline = fit.IntensityY, fit.MuY
	}

	headroom := -1.0
	saturationRadius := gates.SaturationRadius
	frenzyAsymmetry := gates.FrenzyAsymmetry

	if fit.SpectralRadius < saturationRadius {
		margin := saturationRadius - fit.SpectralRadius
		saturation = probability.CompetitionMargin(margin, saturationRadius)

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

	if asymmetry < frenzyAsymmetry {
		margin := frenzyAsymmetry - asymmetry
		frenzy = probability.CompetitionMargin(margin, frenzyAsymmetry)

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

func temperatureScaledGates(gates hawkes.FitGates) hawkes.FitGates {
	if !gates.Ready() {
		return gates
	}

	temperature, ready := market.MacroTemperature()

	if !ready || temperature <= 0 {
		return gates
	}

	scale := viper.GetFloat64("trading.entry.temperature_scale")

	if scale <= 0 {
		scale = 0.35
	}

	scaled := gates
	scaled.FrenzyAsymmetry = math.Min(
		1,
		gates.FrenzyAsymmetry*(1+scale*temperature),
	)

	return scaled
}
