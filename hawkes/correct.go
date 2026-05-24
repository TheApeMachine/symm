package hawkes

/*
applyExcitationCalibration scales buy-side excitation parameters in a stored fit.
Calibration is the running mean of actualReturn/predictedReturn from settled forecasts.
*/
func applyExcitationCalibration(fit BivariateFit, calibration float64) BivariateFit {
	if !fit.valid() {
		return fit
	}

	if calibration <= 0 {
		return zeroBuyExcitationPrior(fit)
	}

	if calibration == 1 {
		return fit
	}

	scaled := fit
	scaled.AlphaBB *= calibration
	scaled.AlphaBS *= calibration
	scaled.SpectralRadius = spectralRadius(
		scaled.AlphaBB, scaled.AlphaBS, scaled.AlphaSB, scaled.AlphaSS, scaled.Beta,
	)

	if scaled.SpectralRadius >= criticalBranch {
		scaled = clampAlphasSubcritical(scaled)
	}

	if !scaled.valid() {
		return fit
	}

	excitation := fit.BuyIntensity - fit.MuBuy

	if excitation > 0 {
		scaled.BuyIntensity = fit.MuBuy + excitation*calibration
	}

	return scaled
}

func zeroBuyExcitationPrior(fit BivariateFit) BivariateFit {
	zeroed := fit
	zeroed.AlphaBB = 0
	zeroed.AlphaBS = 0
	zeroed.BuyIntensity = fit.MuBuy
	zeroed.SpectralRadius = spectralRadius(
		zeroed.AlphaBB, zeroed.AlphaBS, zeroed.AlphaSB, zeroed.AlphaSS, zeroed.Beta,
	)

	return zeroed
}

func clampAlphasSubcritical(fit BivariateFit) BivariateFit {
	if fit.SpectralRadius <= 0 || fit.SpectralRadius >= criticalBranch {
		return fit
	}

	factor := criticalBranch / fit.SpectralRadius

	if factor >= 1 {
		return fit
	}

	clamped := fit
	clamped.AlphaBB *= factor
	clamped.AlphaBS *= factor
	clamped.AlphaSB *= factor
	clamped.AlphaSS *= factor
	clamped.SpectralRadius = spectralRadius(
		clamped.AlphaBB, clamped.AlphaBS, clamped.AlphaSB, clamped.AlphaSS, clamped.Beta,
	)

	return clamped
}
