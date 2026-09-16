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
func (bivariateFit bivariateFit) valid() bool {
	return bivariateFit.muX > 0 &&
		bivariateFit.muY > 0 &&
		bivariateFit.beta > 0 &&
		bivariateFit.alphaXX >= 0 &&
		bivariateFit.alphaXY >= 0 &&
		bivariateFit.alphaYX >= 0 &&
		bivariateFit.alphaYY >= 0 &&
		bivariateFit.spectralRadius >= 0 &&
		bivariateFit.spectralRadius < criticalBranch
}

func (bivariateFit bivariateFit) branchingMatrix() [2][2]float64 {
	return branchingMatrix(bivariateFit.alphaXX, bivariateFit.alphaXY, bivariateFit.alphaYX, bivariateFit.alphaYY, bivariateFit.beta)
}

func (bivariateFit bivariateFit) computeSpectralRadius() float64 {
	if bivariateFit.beta <= 0 {
		return math.Inf(1)
	}

	return spectralRadius(bivariateFit.branchingMatrix())
}

/*
logLikelihood returns the exact log-likelihood at horizon: the sum of
log-intensities at every observed event, minus the compensator (the
integrated intensity over the observation window).
*/
func (bivariateFit bivariateFit) logLikelihood(stream arrivalStream, horizonSec float64) (float64, bool) {
	if bivariateFit.muX <= 0 || bivariateFit.muY <= 0 || bivariateFit.beta <= 0 {
		return 0, false
	}

	if bivariateFit.alphaXX < 0 || bivariateFit.alphaXY < 0 || bivariateFit.alphaYX < 0 || bivariateFit.alphaYY < 0 {
		return 0, false
	}

	marked := stream.marked

	if len(marked) == 0 {
		return 0, false
	}

	span := stream.span(horizonSec)

	if span <= 0 {
		return 0, false
	}

	state := excitationState{}
	logSum, ok := state.logLikelihoodSum(
		marked,
		stream.originSec, horizonSec,
		bivariateFit.muX, bivariateFit.muY,
		bivariateFit.alphaXX, bivariateFit.alphaXY, bivariateFit.alphaYX, bivariateFit.alphaYY,
		bivariateFit.beta,
	)

	if !ok {
		return 0, false
	}

	compensator := bivariateFit.compensator(stream, horizonSec, span)

	return logSum - compensator, true
}

/*
withIntensitiesAt attaches horizon intensities to the fit.
*/
func (bivariateFit bivariateFit) withIntensitiesAt(stream arrivalStream, horizonSec float64) bivariateFit {
	result := bivariateFit
	result.intensityX = stream.buyIntensityAt(horizonSec, bivariateFit.muX, bivariateFit.alphaXX, bivariateFit.alphaXY, bivariateFit.beta)
	result.intensityY = stream.sellIntensityAt(horizonSec, bivariateFit.muY, bivariateFit.alphaYX, bivariateFit.alphaYY, bivariateFit.beta)

	return result
}

func (bivariateFit bivariateFit) withCrossZeroed() bivariateFit {
	if bivariateFit.alphaXY <= 0 && bivariateFit.alphaYX <= 0 {
		return bivariateFit
	}

	restricted := bivariateFit
	restricted.alphaXY = 0
	restricted.alphaYX = 0
	restricted.spectralRadius = restricted.computeSpectralRadius()

	return restricted
}

/*
compensator returns the integrated intensity over the observation window:
Λ_x(horizon) + Λ_y(horizon), the subtracted term in the Hawkes log-likelihood.
*/
func (bivariateFit bivariateFit) compensator(
	stream arrivalStream,
	horizonSec float64,
	span float64,
) float64 {
	beta := bivariateFit.beta
	buySupport, sellSupport := stream.kernelIntegralSupport(horizonSec, beta)

	buyIntegral := bivariateFit.muX*span +
		(bivariateFit.alphaXX/beta)*buySupport +
		(bivariateFit.alphaXY/beta)*sellSupport
	sellIntegral := bivariateFit.muY*span +
		(bivariateFit.alphaYX/beta)*buySupport +
		(bivariateFit.alphaYY/beta)*sellSupport

	return buyIntegral + sellIntegral
}
