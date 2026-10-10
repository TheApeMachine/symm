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
	path    *path
	dropped error
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
Step incorporates one arrival event and returns the Hawkes metrics that are
defined for it, keyed by output name, plus the observation window's origin.
A metric that is undefined for this event (no fitted model yet, zero span,
an unscorable likelihood) is absent, never zero.
*/
func (h *Hawkes) Step(mark, atSec float64) (map[string]float64, time.Time, error) {
	p := h.path
	at := time.Unix(0, int64(atSec*1e9))

	if p.hasLast && at.Before(p.lastAt) {
		return nil, time.Time{}, errnie.Error(errnie.Err(
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
	from := at
	fromSec := atSec

	if len(p.samples) > 0 {
		from = p.origin()
		fromSec = p.samples[0].atSec
	}

	span := atSec - fromSec

	res := map[string]float64{
		"event_count":         totalCount,
		"event_count:buy":     countBuy,
		"event_count:sell":    countSell,
		"event_fraction:buy":  countBuy / totalCount,
		"event_fraction:sell": countSell / totalCount,
	}

	if span > 0 {
		res["arrival_rate:buy"] = countBuy / span
		res["arrival_rate:sell"] = countSell / span
		res["arrival_rate"] = totalCount / span
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

		res["conditional_intensity:buy"] = lambdaBuy
		res["conditional_intensity:sell"] = lambdaSell
		res["conditional_intensity"] = lambdaBuy + lambdaSell
		res["background_rate:buy"] = muX
		res["background_rate:sell"] = muY
		res["background_rate"] = muX + muY
		res["excitation_intensity:buy"] = excessBuy
		res["excitation_intensity:sell"] = excessSell

		if lambdaBuy > 0 {
			res["excitation_fraction:buy"] = excessBuy / lambdaBuy
		}

		if lambdaSell > 0 {
			res["excitation_fraction:sell"] = excessSell / lambdaSell
		}

		res["excitation_amplitude:buy_from_buy"] = alphaXX
		res["excitation_amplitude:buy_from_sell"] = alphaXY
		res["excitation_amplitude:sell_from_buy"] = alphaYX
		res["excitation_amplitude:sell_from_sell"] = alphaYY

		if beta > 0 {
			res["excitation_decay"] = beta
			res["excitation_timescale"] = 1.0 / beta
		}

		matrix := branchingMatrix(alphaXX, alphaXY, alphaYX, alphaYY, beta)
		res["offspring:buy_from_buy"] = matrix[0][0]
		res["offspring:buy_from_sell"] = matrix[0][1]
		res["offspring:sell_from_buy"] = matrix[1][0]
		res["offspring:sell_from_sell"] = matrix[1][1]
		res["branching_spectral_radius"] = spectralRadius(matrix)

		buyParent, sellParent, hasDesc := totalDescendants(alphaXX, alphaXY, alphaYX, alphaYY, beta)

		if hasDesc {
			res["expected_descendants_from_buy"] = buyParent
			res["expected_descendants_from_sell"] = sellParent
		}

		streamPrior := newArrivalStream(buyArrivals, sellArrivals)
		spanPrior := streamPrior.span(atSec)

		if spanPrior > 0 {
			streamWindow := currentWindowStream(buyArrivals, sellArrivals, atSec, mark)
			markedCount := float64(len(streamWindow.marked))

			hawkesLL, hawkesOK := model.logLikelihood(streamWindow, atSec)

			if hawkesOK {
				res["log_likelihood:hawkes"] = hawkesLL

				if markedCount > 0 {
					res["log_likelihood_per_event:hawkes"] = hawkesLL / markedCount
				}
			}

			poissonLL, poissonOK := poissonLogLikelihood(streamWindow, atSec)

			if poissonOK {
				res["log_likelihood:poisson"] = poissonLL
			}

			if hawkesOK && poissonOK {
				gainPoisson := hawkesLL - poissonLL
				res["log_likelihood_gain_vs_poisson"] = gainPoisson

				if markedCount > 0 {
					res["log_likelihood_gain_per_event_vs_poisson"] = gainPoisson / markedCount
				}
			}

			if hawkesOK && p.selfOnlyReady {
				selfLL, selfOK := p.selfOnlyModel.logLikelihood(streamWindow, atSec)

				if selfOK {
					gainSelf := hawkesLL - selfLL
					res["log_likelihood:self_only"] = selfLL
					res["log_likelihood_gain_vs_self_only"] = gainSelf

					if markedCount > 0 {
						res["log_likelihood_gain_per_event_vs_self_only"] = gainSelf / markedCount
					}
				}
			}

			// The excitation mass is the kernel part of the compensator, taken
			// directly from the closed-form kernel integral terms. Because alpha >= 0,
			// beta > 0, and the exponential decay kernel integral support is strictly
			// non-negative, the excitation mass is non-negative by construction without
			// relying on backward subtraction from the total compensator.
			buySupport, sellSupport := streamPrior.kernelIntegralSupport(atSec, beta)
			excessBuyMass := (alphaXX/beta)*buySupport + (alphaXY/beta)*sellSupport
			excessSellMass := (alphaYX/beta)*buySupport + (alphaYY/beta)*sellSupport
			compBuy := muX*spanPrior + excessBuyMass
			compSell := muY*spanPrior + excessSellMass

			// The compensator integrates over (origin, at]; arrivals at the
			// origin are prehistory there, so they are not observations here.
			observedBuy, observedSell := streamPrior.observationCounts(atSec)
			innoBuy := float64(observedBuy) - compBuy
			innoSell := float64(observedSell) - compSell

			res["compensator:buy"] = compBuy
			res["compensator:sell"] = compSell
			res["count_innovation:buy"] = innoBuy
			res["count_innovation:sell"] = innoSell

			if compBuy > 0 {
				res["standardized_innovation:buy"] = innoBuy / math.Sqrt(compBuy)
			}

			if compSell > 0 {
				res["standardized_innovation:sell"] = innoSell / math.Sqrt(compSell)
			}

			res["excitation_mass:buy"] = excessBuyMass
			res["excitation_mass:sell"] = excessSellMass

			if compBuy > 0 {
				res["excitation_share:buy"] = excessBuyMass / compBuy
			}

			if compSell > 0 {
				res["excitation_share:sell"] = excessSellMass / compSell
			}

			if compTotal := compBuy + compSell; compTotal > 0 {
				res["excitation_share"] = (excessBuyMass + excessSellMass) / compTotal
			}
		}
	}

	p.remember(at, atSec, mark)

	// refit reports only the transition from a published model to none, so
	// the caller, which knows the symbol, logs it once per transition.
	if err := p.refit(atSec); err != nil {
		h.dropped = err
	}

	return res, from, nil
}

/*
Dropped returns, once, why the last Step discarded the published model, or
nil when no model was discarded since the previous call.
*/
func (h *Hawkes) Dropped() error {
	err := h.dropped
	h.dropped = nil
	return err
}

/*
poissonLogLikelihood scores the stream under its own homogeneous Poisson
maximum-likelihood fit, mu_side = N_side / T over the observation interval
(origin, horizon], which gives N_buy*log(N_buy/T) - N_buy + N_sell*log(N_sell/T)
- N_sell. It is undefined when either side has no counted arrival.
*/
func poissonLogLikelihood(stream arrivalStream, horizonSec float64) (float64, bool) {
	context, ok := newObservationContext(stream, horizonSec)

	if !ok {
		return 0, false
	}

	poisson := context.poissonFit()

	if !poisson.valid() {
		return 0, false
	}

	return poisson.logLikelihood(stream, horizonSec)
}

func currentWindowStream(buy, sell []float64, horizonSec, mark float64) arrivalStream {
	if mark > 0 {
		return newArrivalStream(append(sortedCopy(buy), horizonSec), sortedCopy(sell))
	}

	return newArrivalStream(sortedCopy(buy), append(sortedCopy(sell), horizonSec))
}
