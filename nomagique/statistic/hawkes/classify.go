package hawkes

import "errors"

/*
fitCategory names the dominant Hawkes regime for a fitted process.
*/
type fitCategory int

const (
	fitCategoryOrganic fitCategory = iota
	fitCategoryFrenzy
	fitCategorySaturation
	fitCategoryExhaustion
)

const uniformFitConfidence = 1.0 / 4.0

/*
classifyFit maps a fit and asymmetry to a category and confidence score.
Classification is withheld until fit gates are ready.
*/
func classifyFit(
	fit bivariateFit,
	asymmetry float64,
	preferY bool,
	gates fitGates,
) (category fitCategory, confidence float64, err error) {
	if !gates.ready() {
		return fitCategoryOrganic, 0, errors.New("hawkes: fit gates are not ready")
	}

	return classifyFitWithGates(fit, asymmetry, preferY, gates)
}

func classifyFitWithGates(
	fit bivariateFit,
	asymmetry float64,
	preferY bool,
	gates fitGates,
) (category fitCategory, confidence float64, err error) {
	saturationRadius := gates.saturationRadius
	frenzyAsymmetry := gates.frenzyAsymmetry
	intensity, baseline := fit.intensityX, fit.muX

	if preferY {
		intensity, baseline = fit.intensityY, fit.muY
	}

	switch {
	case asymmetry > frenzyAsymmetry:
		margin := asymmetry - frenzyAsymmetry
		span := 1 - frenzyAsymmetry

		if margin <= 0 || span <= 0 {
			return fitCategoryFrenzy, uniformFitConfidence, nil
		}

		confidence, err = competitionMargin(margin, span)

		if err != nil {
			return fitCategoryOrganic, 0, err
		}

		return fitCategoryFrenzy, confidence, nil
	case fit.spectralRadius >= saturationRadius:
		margin := fit.spectralRadius - saturationRadius
		span := 1 - saturationRadius

		if margin <= 0 || span <= 0 {
			return fitCategorySaturation, uniformFitConfidence, nil
		}

		confidence, err = competitionMargin(margin, span)

		if err != nil {
			return fitCategoryOrganic, 0, err
		}

		return fitCategorySaturation, confidence, nil
	case baseline > 0 && intensity < baseline:
		margin := baseline - intensity

		if margin <= 0 {
			return fitCategoryExhaustion, uniformFitConfidence, nil
		}

		confidence, err = competitionMargin(margin, baseline)

		if err != nil {
			return fitCategoryOrganic, 0, err
		}

		return fitCategoryExhaustion, confidence, nil
	default:
		headroom := -1.0

		if fit.spectralRadius < saturationRadius {
			margin := saturationRadius - fit.spectralRadius
			saturationHead, err := competitionMargin(margin, saturationRadius)

			if err != nil {
				return fitCategoryOrganic, 0, err
			}

			if saturationHead > headroom {
				headroom = saturationHead
			}
		}

		if baseline > 0 && intensity >= baseline && asymmetry < frenzyAsymmetry {
			margin := intensity - baseline
			organicHead := margin / (margin + baseline)

			if organicHead > headroom {
				headroom = organicHead
			}
		}

		if asymmetry < frenzyAsymmetry {
			margin := frenzyAsymmetry - asymmetry
			frenzyHead, err := competitionMargin(margin, frenzyAsymmetry)

			if err != nil {
				return fitCategoryOrganic, 0, err
			}

			if frenzyHead > headroom {
				headroom = frenzyHead
			}
		}

		if headroom < 0 {
			return fitCategoryOrganic, uniformFitConfidence, nil
		}

		return fitCategoryOrganic, headroom, nil
	}
}
