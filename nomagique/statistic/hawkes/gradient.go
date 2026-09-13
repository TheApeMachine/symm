package hawkes

import "math"

type likelihoodGradient struct {
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
func (fit bivariateFit) logLikelihoodGradient(
	stream arrivalStream,
	horizonSec float64,
) (logLikelihood float64, gradient [bivariateParamCount]float64, ok bool) {
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
	buySupportBeta := kernelIntegralSupportBetaDerivative(stream.buy, stream.originSec, horizonSec, fit.beta)
	sellSupportBeta := kernelIntegralSupportBetaDerivative(stream.sell, stream.originSec, horizonSec, fit.beta)
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

func (fit bivariateFit) eventLogLikelihoodGradient(
	marked []markedEvent,
	originSec, horizonSec float64,
	beta float64,
) likelihoodGradient {
	result := likelihoodGradient{valid: true}
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
			decayFactor := expNeg(beta, age)
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
				case sideBuy:
					lambda := fit.muX + fit.alphaXX*buySupport + fit.alphaXY*sellSupport

					if lambda <= 0 {
						return likelihoodGradient{}
					}

					inverse := 1 / lambda
					lambdaBeta := fit.alphaXX*dBuySupport + fit.alphaXY*dSellSupport
					result.logSum += math.Log(lambda)
					result.muX += inverse
					result.alphaXX += inverse * buySupport
					result.alphaXY += inverse * sellSupport
					result.beta += inverse * lambdaBeta
				case sideSell:
					lambda := fit.muY + fit.alphaYX*buySupport + fit.alphaYY*sellSupport

					if lambda <= 0 {
						return likelihoodGradient{}
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
			case sideBuy:
				buySupport += 1
			case sideSell:
				sellSupport += 1
			}
		}

		index = end
	}

	return result
}

func (fit bivariateFit) compensatorBetaDerivative(
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
logSpaceGradient converts a natural-parameter gradient into the log-space
(unconstrained optimizer) gradient via the chain rule, since the optimizer
searches over log(muX), log(muY), log(beta), and log-branch coordinates
rather than the natural parameters directly.
*/
func logSpaceGradient(naturalGradient [bivariateParamCount]float64, fit bivariateFit) [bivariateParamCount]float64 {
	alphaContribution := naturalGradient[2]*fit.alphaXX +
		naturalGradient[3]*fit.alphaXY +
		naturalGradient[4]*fit.alphaYX +
		naturalGradient[5]*fit.alphaYY

	return [bivariateParamCount]float64{
		naturalGradient[0] * fit.muX,
		naturalGradient[1] * fit.muY,
		naturalGradient[6]*fit.beta + alphaContribution,
		naturalGradient[2] * fit.alphaXX,
		naturalGradient[3] * fit.alphaXY,
		naturalGradient[4] * fit.alphaYX,
		naturalGradient[5] * fit.alphaYY,
	}
}
