package hawkes

import "math"

/*
logPositiveFloor keeps LogPositive off ln(0): a Hawkes log-likelihood sums
ln(lambda) at every event, and a single non-positive intensity would make
that sum negative infinity rather than surfacing as a rejected candidate
during optimization.
*/
const logPositiveFloor = 1e-300

/*
expNeg returns exp(-beta * age), the exponential-kernel decay factor.
*/
func expNeg(beta, age float64) float64 {
	return math.Exp(-beta * age)
}

/*
logPositive returns ln(value), flooring non-positive inputs.
*/
func logPositive(value float64) float64 {
	if value <= logPositiveFloor {
		value = logPositiveFloor
	}

	return math.Log(value)
}

/*
excitationSum sums exp(-beta * remaining) over event ages before horizon: the
raw kernel support one side contributes to the conditional intensity at
horizon.
*/
func excitationSum(eventTimesSec []float64, horizonSec float64, beta float64) float64 {
	sum := 0.0

	for _, eventTime := range eventTimesSec {
		if eventTime > horizonSec {
			continue
		}

		remaining := horizonSec - eventTime

		if remaining > 0 {
			sum += expNeg(beta, remaining)
		}
	}

	return sum
}

/*
intensityAt evaluates mu plus alphaOnBuy*sum(buy impulses) plus
alphaOnSell*sum(sell impulses) at horizon.
*/
func intensityAt(
	buyTimesSec, sellTimesSec []float64,
	horizonSec float64,
	mu, alphaOnBuy, alphaOnSell, beta float64,
) float64 {
	intensity := mu
	intensity += excitationSum(buyTimesSec, horizonSec, beta) * alphaOnBuy
	intensity += excitationSum(sellTimesSec, horizonSec, beta) * alphaOnSell

	return intensity
}

/*
observationKernelIntegralSupport sums exp(-beta*lowerAge) - exp(-beta*upperAge)
over event ages: the closed-form integral of the exponential kernel from the
observation origin to horizon, per event, used by the compensator.
*/
func observationKernelIntegralSupport(
	eventTimesSec []float64,
	originSec, horizonSec float64,
	beta float64,
) float64 {
	support := 0.0

	for _, eventTime := range eventTimesSec {
		if eventTime > horizonSec {
			break
		}

		lowerAge := originSec - eventTime

		if lowerAge < 0 {
			lowerAge = 0
		}

		upperAge := horizonSec - eventTime

		if upperAge <= lowerAge {
			continue
		}

		support += expNeg(beta, lowerAge) - expNeg(beta, upperAge)
	}

	return support
}

/*
kernelIntegralSupportBetaDerivative returns d/dbeta of
observationKernelIntegralSupport, needed by the analytical log-likelihood
gradient with respect to the decay rate.
*/
func kernelIntegralSupportBetaDerivative(
	eventTimesSec []float64,
	originSec, horizonSec float64,
	beta float64,
) float64 {
	derivative := 0.0

	for _, eventTime := range eventTimesSec {
		if eventTime > horizonSec {
			break
		}

		lowerAge := originSec - eventTime

		if lowerAge < 0 {
			lowerAge = 0
		}

		upperAge := horizonSec - eventTime

		if upperAge > lowerAge {
			derivative += upperAge*expNeg(beta, upperAge) - lowerAge*expNeg(beta, lowerAge)
		}
	}

	return derivative
}
