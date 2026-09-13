package hawkes

import "math"

/*
bivariateFit holds joint Hawkes MLE parameters and horizon intensities for
one bivariate exponential-kernel process.
*/
type bivariateFit struct {
	muX            float64
	muY            float64
	alphaXX        float64
	alphaXY        float64
	alphaYX        float64
	alphaYY        float64
	beta           float64
	intensityX     float64
	intensityY     float64
	spectralRadius float64
}

/*
valid reports whether fit parameters are positive and subcritical.
*/
func (fit bivariateFit) valid() bool {
	return fit.muX > 0 &&
		fit.muY > 0 &&
		fit.beta > 0 &&
		fit.alphaXX >= 0 &&
		fit.alphaXY >= 0 &&
		fit.alphaYX >= 0 &&
		fit.alphaYY >= 0 &&
		fit.spectralRadius >= 0 &&
		fit.spectralRadius < criticalBranch
}

func (fit bivariateFit) branchingMatrix() [2][2]float64 {
	return branchingMatrix(fit.alphaXX, fit.alphaXY, fit.alphaYX, fit.alphaYY, fit.beta)
}

func (fit bivariateFit) computeSpectralRadius() float64 {
	if fit.beta <= 0 {
		return math.Inf(1)
	}

	return spectralRadius(fit.branchingMatrix())
}

/*
logLikelihood returns the exact log-likelihood at horizon: the sum of
log-intensities at every observed event, minus the compensator (the
integrated intensity over the observation window).
*/
func (fit bivariateFit) logLikelihood(stream arrivalStream, horizonSec float64) float64 {
	if fit.muX <= 0 || fit.muY <= 0 || fit.beta <= 0 {
		return math.Inf(-1)
	}

	if fit.alphaXX < 0 || fit.alphaXY < 0 || fit.alphaYX < 0 || fit.alphaYY < 0 {
		return math.Inf(-1)
	}

	marked := stream.marked

	if len(marked) == 0 {
		return math.Inf(-1)
	}

	span := stream.span(horizonSec)

	if span <= 0 {
		return math.Inf(-1)
	}

	state := excitationState{}
	logSum, ok := state.logLikelihoodSum(
		marked,
		stream.originSec, horizonSec,
		fit.muX, fit.muY,
		fit.alphaXX, fit.alphaXY, fit.alphaYX, fit.alphaYY,
		fit.beta,
	)

	if !ok {
		return math.Inf(-1)
	}

	compensator := fit.compensator(stream, horizonSec, span)

	return logSum - compensator
}

/*
withIntensitiesAt attaches horizon intensities to the fit.
*/
func (fit bivariateFit) withIntensitiesAt(stream arrivalStream, horizonSec float64) bivariateFit {
	result := fit
	result.intensityX = stream.buyIntensityAt(horizonSec, fit.muX, fit.alphaXX, fit.alphaXY, fit.beta)
	result.intensityY = stream.sellIntensityAt(horizonSec, fit.muY, fit.alphaYX, fit.alphaYY, fit.beta)

	return result
}

func (fit bivariateFit) withCrossZeroed() bivariateFit {
	if fit.alphaXY <= 0 && fit.alphaYX <= 0 {
		return fit
	}

	restricted := fit
	restricted.alphaXY = 0
	restricted.alphaYX = 0
	restricted.spectralRadius = restricted.computeSpectralRadius()

	return restricted
}

/*
compensator returns the integrated intensity over the observation window:
Λ_x(horizon) + Λ_y(horizon), the subtracted term in the Hawkes log-likelihood.
*/
func (fit bivariateFit) compensator(
	stream arrivalStream,
	horizonSec float64,
	span float64,
) float64 {
	beta := fit.beta
	buySupport, sellSupport := stream.kernelIntegralSupport(horizonSec, beta)

	buyIntegral := fit.muX*span +
		(fit.alphaXX/beta)*buySupport +
		(fit.alphaXY/beta)*sellSupport
	sellIntegral := fit.muY*span +
		(fit.alphaYX/beta)*buySupport +
		(fit.alphaYY/beta)*sellSupport

	return buyIntegral + sellIntegral
}
