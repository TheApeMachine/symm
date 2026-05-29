package hawkes

import (
	"math"
	"time"

	"github.com/theapemachine/symm/numeric/decay"
	"github.com/theapemachine/symm/numeric/timeline"
)

type likelihoodGradient struct {
	muBuy   float64
	muSell  float64
	alphaBB float64
	alphaBS float64
	alphaSB float64
	alphaSS float64
	beta    float64
	logSum  float64
	valid   bool
}

/*
LogLikelihoodGradient returns the log-likelihood and its partial derivatives
with respect to muBuy, muSell, alphaBB, alphaBS, alphaSB, alphaSS, and beta.
*/
func (fit BivariateFit) LogLikelihoodGradient(
	stream ArrivalStream,
	horizon time.Time,
) (logLikelihood float64, gradient [7]float64, ok bool) {
	if fit.MuBuy <= 0 || fit.MuSell <= 0 || fit.Beta <= 0 {
		return math.Inf(-1), gradient, false
	}

	marked := stream.Marked()

	if len(marked) == 0 {
		return math.Inf(-1), gradient, false
	}

	span := stream.Span(horizon)

	if span <= 0 {
		return math.Inf(-1), gradient, false
	}

	eventGradient := fit.eventLogLikelihoodGradient(marked, fit.Beta)

	if !eventGradient.valid {
		return math.Inf(-1), gradient, false
	}

	buySupport, sellSupport := stream.kernelSupport(horizon, fit.Beta)
	buySupportBeta := kernelSupportBetaDerivative(stream.buy, horizon, fit.Beta)
	sellSupportBeta := kernelSupportBetaDerivative(stream.sell, horizon, fit.Beta)
	beta := fit.Beta

	compensator := fit.MuBuy*span +
		(fit.AlphaBB/beta)*buySupport +
		(fit.AlphaBS/beta)*sellSupport +
		fit.MuSell*span +
		(fit.AlphaSB/beta)*buySupport +
		(fit.AlphaSS/beta)*sellSupport

	gradient[0] = eventGradient.muBuy - span
	gradient[1] = eventGradient.muSell - span
	gradient[2] = eventGradient.alphaBB - buySupport/beta
	gradient[3] = eventGradient.alphaBS - sellSupport/beta
	gradient[4] = eventGradient.alphaSB - buySupport/beta
	gradient[5] = eventGradient.alphaSS - sellSupport/beta
	gradient[6] = eventGradient.beta - fit.compensatorBetaDerivative(
		buySupport, sellSupport, buySupportBeta, sellSupportBeta,
	)

	logLikelihood = eventGradient.logSum - compensator

	return logLikelihood, gradient, true
}

func (fit BivariateFit) eventLogLikelihoodGradient(
	marked []markedEvent,
	beta float64,
) likelihoodGradient {
	result := likelihoodGradient{valid: true}
	buyToBuy := 0.0
	sellToBuy := 0.0
	buyToSell := 0.0
	sellToSell := 0.0
	dBuyToBuy := 0.0
	dSellToBuy := 0.0
	dBuyToSell := 0.0
	dSellToSell := 0.0
	lastTime := marked[0].at
	haveLast := true

	for index := 0; index < len(marked); {
		eventTime := marked[index].at

		if haveLast && eventTime.After(lastTime) {
			decayFactor := decay.ExpNeg(beta, eventTime.Sub(lastTime).Seconds())
			age := eventTime.Sub(lastTime).Seconds()
			dBuyToBuy = (dBuyToBuy - buyToBuy*age) * decayFactor
			dSellToBuy = (dSellToBuy - sellToBuy*age) * decayFactor
			dBuyToSell = (dBuyToSell - buyToSell*age) * decayFactor
			dSellToSell = (dSellToSell - sellToSell*age) * decayFactor
			buyToBuy *= decayFactor
			sellToBuy *= decayFactor
			buyToSell *= decayFactor
			sellToSell *= decayFactor
			lastTime = eventTime
		}

		end := index

		for end < len(marked) && marked[end].at.Equal(eventTime) {
			end++
		}

		for _, event := range marked[index:end] {
			switch event.side {
			case sideBuy:
				lambda := fit.MuBuy + fit.AlphaBB*buyToBuy + fit.AlphaBS*sellToBuy

				if lambda <= 0 {
					return likelihoodGradient{}
				}

				inverse := 1 / lambda
				lambdaBeta := fit.AlphaBB*dBuyToBuy + fit.AlphaBS*dSellToBuy
				result.logSum += math.Log(lambda)
				result.muBuy += inverse
				result.alphaBB += inverse * buyToBuy
				result.alphaBS += inverse * sellToBuy
				result.beta += inverse * lambdaBeta
			case sideSell:
				lambda := fit.MuSell + fit.AlphaSB*buyToSell + fit.AlphaSS*sellToSell

				if lambda <= 0 {
					return likelihoodGradient{}
				}

				inverse := 1 / lambda
				lambdaBeta := fit.AlphaSB*dBuyToSell + fit.AlphaSS*dSellToSell
				result.logSum += math.Log(lambda)
				result.muSell += inverse
				result.alphaSB += inverse * buyToSell
				result.alphaSS += inverse * sellToSell
				result.beta += inverse * lambdaBeta
			}
		}

		for _, event := range marked[index:end] {
			switch event.side {
			case sideBuy:
				buyToBuy += 1
				buyToSell += 1
			case sideSell:
				sellToBuy += 1
				sellToSell += 1
			}
		}

		index = end
	}

	return result
}

func kernelSupportBetaDerivative(
	events timeline.Timeline,
	horizon time.Time,
	beta float64,
) float64 {
	var derivative float64

	for _, eventTime := range events.Times() {
		remaining := horizon.Sub(eventTime).Seconds()

		if remaining > 0 {
			derivative += remaining * decay.ExpNeg(beta, remaining)
		}
	}

	return derivative
}

func (fit BivariateFit) compensatorBetaDerivative(
	buySupport, sellSupport, buySupportBeta, sellSupportBeta float64,
) float64 {
	beta := fit.Beta
	branchBuy := fit.AlphaBB / beta
	branchCrossToBuy := fit.AlphaBS / beta
	branchCrossToSell := fit.AlphaSB / beta
	branchSell := fit.AlphaSS / beta

	return -branchBuy/beta*buySupport +
		branchBuy*buySupportBeta +
		-branchCrossToBuy/beta*sellSupport +
		branchCrossToBuy*sellSupportBeta +
		-branchCrossToSell/beta*buySupport +
		branchCrossToSell*buySupportBeta +
		-branchSell/beta*sellSupport +
		branchSell*sellSupportBeta
}

func logSpaceGradient(naturalGradient [7]float64, fit BivariateFit) [7]float64 {
	alphaContribution := naturalGradient[2]*fit.AlphaBB +
		naturalGradient[3]*fit.AlphaBS +
		naturalGradient[4]*fit.AlphaSB +
		naturalGradient[5]*fit.AlphaSS

	return [7]float64{
		naturalGradient[0] * fit.MuBuy,
		naturalGradient[1] * fit.MuSell,
		naturalGradient[6]*fit.Beta + alphaContribution,
		naturalGradient[2] * fit.AlphaBB,
		naturalGradient[3] * fit.AlphaBS,
		naturalGradient[4] * fit.AlphaSB,
		naturalGradient[5] * fit.AlphaSS,
	}
}
