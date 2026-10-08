package hawkes

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
)

/*
Hawkes wraps the streaming bivariate Hawkes process state and estimators.
*/
type Hawkes struct {
	path *path
}

/*
NewHawkes creates a new Hawkes estimator instance.
*/
func NewHawkes() *Hawkes {
	return &Hawkes{
		path: &path{samples: make([]sample, 0, MaxArrivalSamples)},
	}
}

/*
Step incorporates one arrival event and returns all Hawkes metrics.
*/
func (h *Hawkes) Step(mark, atSec float64) ([]float64, error) {
	p := h.path
	at := time.Unix(0, int64(atSec*1e9))

	if p.hasLast && at.Before(p.lastAt) {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"hawkes: regressing event time",
			nil,
		))
	}

	p.lastAt = at
	p.hasLast = true

	buyArrivals, sellArrivals := p.sides()
	countBuy := float64(len(buyArrivals))
	countSell := float64(len(sellArrivals))

	if mark > 0 {
		countBuy++
	}

	if mark <= 0 {
		countSell++
	}

	totalCount := countBuy + countSell
	fromSec := atSec

	if len(p.samples) > 0 {
		fromSec = float64(p.origin().UnixNano()) * 1e-9
	}

	span := atSec - fromSec

	res := make([]float64, 62)
	res[0] = totalCount
	res[1] = countBuy
	res[2] = countSell

	if totalCount > 0 {
		res[3] = countBuy / totalCount
		res[4] = countSell / totalCount
	}

	if span > 0 {
		res[5] = countBuy / span
		res[6] = countSell / span
		res[7] = totalCount / span
	}

	if p.modelReady {
		model := p.model
		muX, muY := model.muX, model.muY
		alphaXX, alphaXY := model.alphaXX, model.alphaXY
		alphaYX, alphaYY := model.alphaYX, model.alphaYY
		beta := model.beta

		lambdaBuy := intensityAt(buyArrivals, sellArrivals, atSec, muX, alphaXX, alphaXY, beta)
		lambdaSell := intensityAt(buyArrivals, sellArrivals, atSec, muY, alphaYX, alphaYY, beta)
		excessBuy := lambdaBuy - muX
		excessSell := lambdaSell - muY

		res[8] = lambdaBuy
		res[9] = lambdaSell
		res[10] = lambdaBuy + lambdaSell
		res[11] = muX
		res[12] = muY
		res[13] = muX + muY
		res[14] = excessBuy
		res[15] = excessSell

		if lambdaBuy > 0 {
			res[16] = excessBuy / lambdaBuy
		}

		if lambdaSell > 0 {
			res[17] = excessSell / lambdaSell
		}

		res[18] = alphaXX
		res[19] = alphaXY
		res[20] = alphaYX
		res[21] = alphaYY

		if beta > 0 {
			timescale := 1.0 / beta
			res[22] = beta
			res[23] = beta
			res[24] = beta
			res[25] = beta
			res[26] = beta
			res[27] = timescale
			res[28] = timescale
			res[29] = timescale
			res[30] = timescale
			res[31] = timescale
		}

		matrix := branchingMatrix(alphaXX, alphaXY, alphaYX, alphaYY, beta)
		res[32] = matrix[0][0]
		res[33] = matrix[0][1]
		res[34] = matrix[1][0]
		res[35] = matrix[1][1]
		res[36] = spectralRadius(matrix)

		buyParent, sellParent, hasDesc := totalDescendants(alphaXX, alphaXY, alphaYX, alphaYY, beta)

		if hasDesc {
			res[37] = buyParent
			res[38] = sellParent
		}

		streamPrior := newArrivalStream(buyArrivals, sellArrivals)
		spanPrior := streamPrior.span(atSec)

		if spanPrior > 0 {
			streamWindow := currentWindowStream(buyArrivals, sellArrivals, atSec, mark)
			markedCount := float64(len(streamWindow.marked))

			hawkesLL, hawkesOK := model.logLikelihood(streamWindow, atSec)

			if hawkesOK {
				res[39] = hawkesLL

				if markedCount > 0 {
					res[40] = hawkesLL / markedCount
				}
			}

			poisson := bivariateFit{muX: muX, muY: muY, beta: beta}
			poissonLL, poissonOK := poisson.logLikelihood(streamWindow, atSec)

			if poissonOK {
				res[41] = poissonLL
			}

			if hawkesOK && poissonOK {
				gainPoisson := hawkesLL - poissonLL
				res[42] = gainPoisson

				if markedCount > 0 {
					res[43] = gainPoisson / markedCount
				}
			}

			if hawkesOK && p.selfOnlyReady {
				selfLL, selfOK := p.selfOnlyModel.logLikelihood(streamWindow, atSec)

				if selfOK {
					gainSelf := hawkesLL - selfLL
					res[44] = selfLL
					res[45] = gainSelf

					if markedCount > 0 {
						res[46] = gainSelf / markedCount
					}
				}
			}

			buySupport, sellSupport := streamPrior.kernelIntegralSupport(atSec, beta)
			compBuy := muX*spanPrior + (alphaXX/beta)*buySupport + (alphaXY/beta)*sellSupport
			compSell := muY*spanPrior + (alphaYX/beta)*buySupport + (alphaYY/beta)*sellSupport

			priorCountBuy := float64(len(buyArrivals))
			priorCountSell := float64(len(sellArrivals))
			innoBuy := priorCountBuy - compBuy
			innoSell := priorCountSell - compSell

			res[47] = compBuy
			res[48] = compSell
			res[49] = innoBuy
			res[50] = innoSell

			if compBuy > 0 {
				res[51] = innoBuy / math.Sqrt(compBuy)
			}

			if compSell > 0 {
				res[52] = innoSell / math.Sqrt(compSell)
			}

			excessBuyMass := compBuy - muX*spanPrior
			excessSellMass := compSell - muY*spanPrior

			res[53] = excessBuyMass
			res[54] = excessSellMass

			if compBuy > 0 {
				res[55] = excessBuyMass / compBuy
			}

			if compSell > 0 {
				res[56] = excessSellMass / compSell
			}

			if compTotal := compBuy + compSell; compTotal > 0 {
				res[57] = (excessBuyMass + excessSellMass) / compTotal
			}

			snrSum := 0.0
			snrSides := 0

			if compBuy > 0 {
				snrSum += (excessBuyMass * excessBuyMass) / compBuy
				snrSides++
			}

			if compSell > 0 {
				snrSum += (excessSellMass * excessSellMass) / compSell
				snrSides++
			}

			if snrSides > 0 {
				p.snr = snrSum / float64(snrSides)
				p.hasSNR = true
				res[58] = p.snr
			}
		}
	}

	res[61] = fromSec

	p.remember(at, atSec, mark)
	p.refit(atSec)

	return res, nil
}

func currentWindowStream(buy, sell []float64, horizonSec, mark float64) arrivalStream {
	if mark > 0 {
		return newArrivalStream(append(sortedCopy(buy), horizonSec), sortedCopy(sell))
	}

	return newArrivalStream(sortedCopy(buy), append(sortedCopy(sell), horizonSec))
}
