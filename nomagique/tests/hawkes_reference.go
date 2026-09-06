// Test-only oracle copied from supplied Hawkes chronological gradient and decay.
package tests

import "math"

type referenceHawkesLikelihoodGradient struct {
	muX     float64
	muY     float64
	alphaXX float64
	alphaXY float64
	alphaYX float64
	alphaYY float64
	beta    float64
	logSum  float64
	valid   bool
}

/*
logLikelihoodGradient returns log-likelihood and partial derivatives at
horizon with respect to (muX, muY, alphaXX, alphaXY, alphaYX, alphaYY, beta),
in that natural-parameter order.
*/
func (fit referenceHawkesBivariateFit) logLikelihoodGradient(
	stream referenceHawkesArrivalStream,
	horizonSec float64,
) (logLikelihood float64, gradient [referenceHawkesBivariateParamCount]float64, ok bool) {
	if fit.muX <= 0 || fit.muY <= 0 || fit.beta <= 0 {
		return math.Inf(-1), gradient, false
	}

	marked := stream.marked

	if len(marked) == 0 {
		return math.Inf(-1), gradient, false
	}

	span := stream.span(horizonSec)

	if span <= 0 {
		return math.Inf(-1), gradient, false
	}

	eventGradient := fit.eventLogLikelihoodGradient(marked, stream.originSec, horizonSec, fit.beta)

	if !eventGradient.valid {
		return math.Inf(-1), gradient, false
	}

	buySupport, sellSupport := stream.kernelIntegralSupport(horizonSec, fit.beta)
	buySupportBeta := referenceHawkesKernelIntegralSupportBetaDerivative(stream.buy, stream.originSec, horizonSec, fit.beta)
	sellSupportBeta := referenceHawkesKernelIntegralSupportBetaDerivative(stream.sell, stream.originSec, horizonSec, fit.beta)
	beta := fit.beta

	compensator := fit.muX*span +
		(fit.alphaXX/beta)*buySupport +
		(fit.alphaXY/beta)*sellSupport +
		fit.muY*span +
		(fit.alphaYX/beta)*buySupport +
		(fit.alphaYY/beta)*sellSupport

	gradient[0] = eventGradient.muX - span
	gradient[1] = eventGradient.muY - span
	gradient[2] = eventGradient.alphaXX - buySupport/beta
	gradient[3] = eventGradient.alphaXY - sellSupport/beta
	gradient[4] = eventGradient.alphaYX - buySupport/beta
	gradient[5] = eventGradient.alphaYY - sellSupport/beta
	gradient[6] = eventGradient.beta - fit.compensatorBetaDerivative(
		buySupport, sellSupport, buySupportBeta, sellSupportBeta,
	)

	logLikelihood = eventGradient.logSum - compensator

	return logLikelihood, gradient, true
}

func (fit referenceHawkesBivariateFit) eventLogLikelihoodGradient(
	marked []referenceHawkesMarkedEvent,
	originSec, horizonSec float64,
	beta float64,
) referenceHawkesLikelihoodGradient {
	result := referenceHawkesLikelihoodGradient{valid: true}
	buySupport := 0.0
	sellSupport := 0.0
	dBuySupport := 0.0
	dSellSupport := 0.0
	lastTime := marked[0].atSec
	haveLast := true

	for index := 0; index < len(marked); {
		eventTime := marked[index].atSec

		if eventTime > horizonSec {
			break
		}

		if haveLast && eventTime > lastTime {
			age := eventTime - lastTime
			decayFactor := referenceHawkesExpNeg(beta, age)
			dBuySupport = (dBuySupport - buySupport*age) * decayFactor
			dSellSupport = (dSellSupport - sellSupport*age) * decayFactor
			buySupport *= decayFactor
			sellSupport *= decayFactor
			lastTime = eventTime
		}

		end := index

		for end < len(marked) && marked[end].atSec == eventTime {
			end++
		}

		if eventTime > originSec {
			for _, event := range marked[index:end] {
				switch event.side {
				case referenceHawkesSideBuy:
					lambda := fit.muX + fit.alphaXX*buySupport + fit.alphaXY*sellSupport

					if lambda <= 0 {
						return referenceHawkesLikelihoodGradient{}
					}

					inverse := 1 / lambda
					lambdaBeta := fit.alphaXX*dBuySupport + fit.alphaXY*dSellSupport
					result.logSum += math.Log(lambda)
					result.muX += inverse
					result.alphaXX += inverse * buySupport
					result.alphaXY += inverse * sellSupport
					result.beta += inverse * lambdaBeta
				case referenceHawkesSideSell:
					lambda := fit.muY + fit.alphaYX*buySupport + fit.alphaYY*sellSupport

					if lambda <= 0 {
						return referenceHawkesLikelihoodGradient{}
					}

					inverse := 1 / lambda
					lambdaBeta := fit.alphaYX*dBuySupport + fit.alphaYY*dSellSupport
					result.logSum += math.Log(lambda)
					result.muY += inverse
					result.alphaYX += inverse * buySupport
					result.alphaYY += inverse * sellSupport
					result.beta += inverse * lambdaBeta
				}
			}
		}

		for _, event := range marked[index:end] {
			switch event.side {
			case referenceHawkesSideBuy:
				buySupport += 1
			case referenceHawkesSideSell:
				sellSupport += 1
			}
		}

		index = end
	}

	return result
}

func (fit referenceHawkesBivariateFit) compensatorBetaDerivative(
	buySupport, sellSupport, buySupportBeta, sellSupportBeta float64,
) float64 {
	beta := fit.beta
	branchX := fit.alphaXX / beta
	branchCrossToX := fit.alphaXY / beta
	branchCrossToY := fit.alphaYX / beta
	branchY := fit.alphaYY / beta

	return -branchX/beta*buySupport +
		branchX*buySupportBeta +
		-branchCrossToX/beta*sellSupport +
		branchCrossToX*sellSupportBeta +
		-branchCrossToY/beta*buySupport +
		branchCrossToY*buySupportBeta +
		-branchY/beta*sellSupport +
		branchY*sellSupportBeta
}

/*
referenceHawkesLogSpaceGradient converts a natural-parameter gradient into the log-space
(unconstrained optimizer) gradient via the chain rule, since the optimizer
searches over log(muX), log(muY), log(beta), and log-branch coordinates
rather than the natural parameters directly.
*/
func referenceHawkesLogSpaceGradient(naturalGradient [referenceHawkesBivariateParamCount]float64, fit referenceHawkesBivariateFit) [referenceHawkesBivariateParamCount]float64 {
	alphaContribution := naturalGradient[2]*fit.alphaXX +
		naturalGradient[3]*fit.alphaXY +
		naturalGradient[4]*fit.alphaYX +
		naturalGradient[5]*fit.alphaYY

	return [referenceHawkesBivariateParamCount]float64{
		naturalGradient[0] * fit.muX,
		naturalGradient[1] * fit.muY,
		naturalGradient[6]*fit.beta + alphaContribution,
		naturalGradient[2] * fit.alphaXX,
		naturalGradient[3] * fit.alphaXY,
		naturalGradient[4] * fit.alphaYX,
		naturalGradient[5] * fit.alphaYY,
	}
}

/*
referenceHawkesLogPositiveFloor keeps LogPositive off ln(0): a Hawkes log-likelihood sums
ln(lambda) at every event, and a single non-positive intensity would make
that sum negative infinity rather than surfacing as a rejected candidate
during optimization.
*/
const referenceHawkesLogPositiveFloor = 1e-300

/*
referenceHawkesExpNeg returns exp(-beta * age), the exponential-kernel decay factor.
*/
func referenceHawkesExpNeg(beta, age float64) float64 {
	return math.Exp(-beta * age)
}

/*
referenceHawkesLogPositive returns ln(value), flooring non-positive inputs.
*/
func referenceHawkesLogPositive(value float64) float64 {
	if value <= referenceHawkesLogPositiveFloor {
		value = referenceHawkesLogPositiveFloor
	}

	return math.Log(value)
}

/*
referenceHawkesExcitationSum sums exp(-beta * remaining) over event ages before horizon: the
raw kernel support one side contributes to the conditional intensity at
horizon.
*/
func referenceHawkesExcitationSum(eventTimesSec []float64, horizonSec float64, beta float64) float64 {
	sum := 0.0

	for _, eventTime := range eventTimesSec {
		if eventTime > horizonSec {
			continue
		}

		remaining := horizonSec - eventTime

		if remaining > 0 {
			sum += referenceHawkesExpNeg(beta, remaining)
		}
	}

	return sum
}

/*
referenceHawkesIntensityAt evaluates mu plus alphaOnBuy*sum(buy impulses) plus
alphaOnSell*sum(sell impulses) at horizon.
*/
func referenceHawkesIntensityAt(
	buyTimesSec, sellTimesSec []float64,
	horizonSec float64,
	mu, alphaOnBuy, alphaOnSell, beta float64,
) float64 {
	intensity := mu
	intensity += referenceHawkesExcitationSum(buyTimesSec, horizonSec, beta) * alphaOnBuy
	intensity += referenceHawkesExcitationSum(sellTimesSec, horizonSec, beta) * alphaOnSell

	return intensity
}

/*
referenceHawkesObservationKernelIntegralSupport sums exp(-beta*lowerAge) - exp(-beta*upperAge)
over event ages: the closed-form integral of the exponential kernel from the
observation origin to horizon, per event, used by the compensator.
*/
func referenceHawkesObservationKernelIntegralSupport(
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

		support += referenceHawkesExpNeg(beta, lowerAge) - referenceHawkesExpNeg(beta, upperAge)
	}

	return support
}

/*
referenceHawkesKernelIntegralSupportBetaDerivative returns d/dbeta of
referenceHawkesObservationKernelIntegralSupport, needed by the analytical log-likelihood
gradient with respect to the decay rate.
*/
func referenceHawkesKernelIntegralSupportBetaDerivative(
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
			derivative += upperAge*referenceHawkesExpNeg(beta, upperAge) - lowerAge*referenceHawkesExpNeg(beta, lowerAge)
		}
	}

	return derivative
}

const referenceHawkesBivariateParamCount = 7
const referenceHawkesSideBuy = 0
const referenceHawkesSideSell = 1

type referenceHawkesMarkedEvent struct {
	atSec float64
	side  int
}
type referenceHawkesBivariateFit struct{ muX, muY, alphaXX, alphaXY, alphaYX, alphaYY, beta float64 }
type referenceHawkesArrivalStream struct {
	marked    []referenceHawkesMarkedEvent
	buy, sell []float64
	originSec float64
}

func (s referenceHawkesArrivalStream) span(horizon float64) float64 { return horizon - s.originSec }
func (s referenceHawkesArrivalStream) kernelIntegralSupport(horizon, beta float64) (float64, float64) {
	return referenceHawkesObservationKernelIntegralSupport(s.buy, s.originSec, horizon, beta), referenceHawkesObservationKernelIntegralSupport(s.sell, s.originSec, horizon, beta)
}
