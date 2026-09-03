package hawkes

import (
	"errors"
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
)

const MaxArrivalSamples = 64

func finite(values ...float64) bool {
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

type arrivalSample struct {
	timestamp time.Time
	atSec     float64
	mark      float64
}

/*
Engine executes the causal, online bivariate Hawkes arrival process model.
It maintains bounded arrival history and refits MLE parameters as data warrants.
*/
type Engine struct {
	samples        []arrivalSample
	lastTimestamp  time.Time
	hasLast        bool
	model          bivariateFit
	modelReady     bool
	selfOnlyModel  bivariateFit
	selfOnlyReady  bool
	eventsSinceFit int
	modelSupport   float64
}

/*
NewEngine initializes an empty online Hawkes arrival engine.
*/
func NewEngine() *Engine {
	return &Engine{
		samples: make([]arrivalSample, 0, MaxArrivalSamples),
	}
}

/*
Step evaluates the arriving event against the previously committed Hawkes model,
projects all empirical and fitted metrics, incorporates the event, and conditionally refits.
*/
func (e *Engine) Step(
	mark float64,
	at time.Time,
	measurement *data.Measurement[float64],
) (time.Time, error) {
	if mark == 0 || !finite(mark) {
		return time.Time{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"hawkes: a finite non-zero mark is required",
			errors.New("invalid mark"),
		))
	}

	if e.hasLast && at.Before(e.lastTimestamp) {
		return time.Time{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"hawkes: regressing event time",
			errors.New("event timestamp is before previous timestamp"),
		))
	}
	e.lastTimestamp = at
	e.hasLast = true

	buyArrivals := make([]float64, 0, len(e.samples))
	sellArrivals := make([]float64, 0, len(e.samples))
	for _, s := range e.samples {
		if s.mark > 0 {
			buyArrivals = append(buyArrivals, s.atSec)
		} else {
			sellArrivals = append(sellArrivals, s.atSec)
		}
	}

	countBuy := float64(len(buyArrivals))
	countSell := float64(len(sellArrivals))
	if mark > 0 {
		countBuy++
	} else {
		countSell++
	}
	count := countBuy + countSell

	from := at
	if len(e.samples) > 0 {
		from = e.samples[0].timestamp
	}

	atSec := float64(at.UnixNano()) * 1e-9
	fromSec := float64(from.UnixNano()) * 1e-9
	span := atSec - fromSec

	putMetric(measurement, "event_count", count, data.UnitCount, data.TimescaleInstantaneous)
	putMetric(measurement, "event_count:buy", countBuy, data.UnitCount, data.TimescaleInstantaneous)
	putMetric(measurement, "event_count:sell", countSell, data.UnitCount, data.TimescaleInstantaneous)
	putMetric(measurement, "event_fraction:buy", countBuy/count, data.UnitDimensionless, data.TimescaleInstantaneous)
	putMetric(measurement, "event_fraction:sell", countSell/count, data.UnitDimensionless, data.TimescaleInstantaneous)

	if span > 0 {
		rateBuy := countBuy / span
		rateSell := countSell / span
		putMetric(measurement, "arrival_rate:buy", rateBuy, data.UnitPerSecond, data.TimescalePerSecond)
		putMetric(measurement, "arrival_rate:sell", rateSell, data.UnitPerSecond, data.TimescalePerSecond)
		putMetric(measurement, "arrival_rate", rateBuy+rateSell, data.UnitPerSecond, data.TimescalePerSecond)
	}

	// 1. Pre-arrival model evaluations
	if e.modelReady {
		muX := e.model.muX
		muY := e.model.muY
		alphaXX := e.model.alphaXX
		alphaXY := e.model.alphaXY
		alphaYX := e.model.alphaYX
		alphaYY := e.model.alphaYY
		beta := e.model.beta

		lambdaBuy := intensityAt(buyArrivals, sellArrivals, atSec, muX, alphaXX, alphaXY, beta)
		lambdaSell := intensityAt(buyArrivals, sellArrivals, atSec, muY, alphaYX, alphaYY, beta)

		if finite(lambdaBuy, lambdaSell) {
			putMetric(measurement, "conditional_intensity:buy", lambdaBuy, data.UnitPerSecond, data.TimescalePerSecond)
			putMetric(measurement, "conditional_intensity:sell", lambdaSell, data.UnitPerSecond, data.TimescalePerSecond)
			putMetric(measurement, "conditional_intensity", lambdaBuy+lambdaSell, data.UnitPerSecond, data.TimescalePerSecond)
			putMetric(measurement, "background_rate:buy", muX, data.UnitPerSecond, data.TimescalePerSecond)
			putMetric(measurement, "background_rate:sell", muY, data.UnitPerSecond, data.TimescalePerSecond)
			putMetric(measurement, "background_rate", muX+muY, data.UnitPerSecond, data.TimescalePerSecond)

			excessBuy := lambdaBuy - muX
			excessSell := lambdaSell - muY
			putMetric(measurement, "excitation_intensity:buy", excessBuy, data.UnitPerSecond, data.TimescalePerSecond)
			putMetric(measurement, "excitation_intensity:sell", excessSell, data.UnitPerSecond, data.TimescalePerSecond)

			if lambdaBuy > 0 {
				putMetric(measurement, "excitation_fraction:buy", excessBuy/lambdaBuy, data.UnitDimensionless, data.TimescaleInstantaneous)
			}
			if lambdaSell > 0 {
				putMetric(measurement, "excitation_fraction:sell", excessSell/lambdaSell, data.UnitDimensionless, data.TimescaleInstantaneous)
			}

			putMetric(measurement, "excitation_amplitude:buy_from_buy", alphaXX, data.UnitDimensionless, data.TimescaleInstantaneous)
			putMetric(measurement, "excitation_amplitude:buy_from_sell", alphaXY, data.UnitDimensionless, data.TimescaleInstantaneous)
			putMetric(measurement, "excitation_amplitude:sell_from_buy", alphaYX, data.UnitDimensionless, data.TimescaleInstantaneous)
			putMetric(measurement, "excitation_amplitude:sell_from_sell", alphaYY, data.UnitDimensionless, data.TimescaleInstantaneous)

			if beta > 0 {
				putMetric(measurement, "excitation_decay", beta, data.UnitPerSecond, data.TimescalePerSecond)
				putMetric(measurement, "excitation_decay:buy_from_buy", beta, data.UnitPerSecond, data.TimescalePerSecond)
				putMetric(measurement, "excitation_decay:buy_from_sell", beta, data.UnitPerSecond, data.TimescalePerSecond)
				putMetric(measurement, "excitation_decay:sell_from_buy", beta, data.UnitPerSecond, data.TimescalePerSecond)
				putMetric(measurement, "excitation_decay:sell_from_sell", beta, data.UnitPerSecond, data.TimescalePerSecond)

				timescale := 1.0 / beta
				putMetric(measurement, "excitation_timescale", timescale, data.UnitSecond, data.TimescaleInstantaneous)
				putMetric(measurement, "excitation_timescale:buy_from_buy", timescale, data.UnitSecond, data.TimescaleInstantaneous)
				putMetric(measurement, "excitation_timescale:buy_from_sell", timescale, data.UnitSecond, data.TimescaleInstantaneous)
				putMetric(measurement, "excitation_timescale:sell_from_buy", timescale, data.UnitSecond, data.TimescaleInstantaneous)
				putMetric(measurement, "excitation_timescale:sell_from_sell", timescale, data.UnitSecond, data.TimescaleInstantaneous)
			}
		}

		// Branching
		matrix := branchingMatrix(alphaXX, alphaXY, alphaYX, alphaYY, beta)
		if finite(matrix[0][0], matrix[0][1], matrix[1][0], matrix[1][1]) {
			putMetric(measurement, "offspring:buy_from_buy", matrix[0][0], data.UnitDimensionless, data.TimescaleInstantaneous)
			putMetric(measurement, "offspring:buy_from_sell", matrix[0][1], data.UnitDimensionless, data.TimescaleInstantaneous)
			putMetric(measurement, "offspring:sell_from_buy", matrix[1][0], data.UnitDimensionless, data.TimescaleInstantaneous)
			putMetric(measurement, "offspring:sell_from_sell", matrix[1][1], data.UnitDimensionless, data.TimescaleInstantaneous)
			putMetric(measurement, "branching_spectral_radius", spectralRadius(matrix), data.UnitDimensionless, data.TimescaleInstantaneous)

			buyParent, sellParent, hasDesc := totalDescendants(alphaXX, alphaXY, alphaYX, alphaYY, beta)
			if hasDesc {
				putMetric(measurement, "expected_descendants_from_buy", buyParent, data.UnitCount, data.TimescaleInstantaneous)
				putMetric(measurement, "expected_descendants_from_sell", sellParent, data.UnitCount, data.TimescaleInstantaneous)
			}
		}

		// Likelihood
		streamWindow := currentWindowStream(buyArrivals, sellArrivals, atSec, mark)
		hawkesLL := e.model.logLikelihood(streamWindow, atSec)
		poisson := bivariateFit{muX: muX, muY: muY, beta: beta}
		poissonLL := poisson.logLikelihood(streamWindow, atSec)

		if finite(hawkesLL, poissonLL) && len(streamWindow.marked) > 0 {
			markedCount := float64(len(streamWindow.marked))
			putMetric(measurement, "log_likelihood:hawkes", hawkesLL, data.UnitNat, data.TimescaleInstantaneous)
			putMetric(measurement, "log_likelihood:poisson", poissonLL, data.UnitNat, data.TimescaleInstantaneous)
			putMetric(measurement, "log_likelihood_per_event:hawkes", hawkesLL/markedCount, data.UnitNat, data.TimescaleInstantaneous)
			putMetric(measurement, "log_likelihood_gain_vs_poisson", hawkesLL-poissonLL, data.UnitNat, data.TimescaleInstantaneous)
			putMetric(measurement, "log_likelihood_gain_per_event_vs_poisson", (hawkesLL-poissonLL)/markedCount, data.UnitNat, data.TimescaleInstantaneous)

			if e.selfOnlyReady {
				selfLL := e.selfOnlyModel.logLikelihood(streamWindow, atSec)
				if finite(selfLL) {
					putMetric(measurement, "log_likelihood:self_only", selfLL, data.UnitNat, data.TimescaleInstantaneous)
					putMetric(measurement, "log_likelihood_gain_vs_self_only", hawkesLL-selfLL, data.UnitNat, data.TimescaleInstantaneous)
					putMetric(measurement, "log_likelihood_gain_per_event_vs_self_only", (hawkesLL-selfLL)/markedCount, data.UnitNat, data.TimescaleInstantaneous)
				}
			}
		}

		// Compensator
		streamPrior := newArrivalStream(buyArrivals, sellArrivals)
		spanPrior := streamPrior.span(atSec)
		if spanPrior > 0 {
			buySupport, sellSupport := streamPrior.kernelIntegralSupport(atSec, beta)
			compBuy := muX*spanPrior + (alphaXX/beta)*buySupport + (alphaXY/beta)*sellSupport
			compSell := muY*spanPrior + (alphaYX/beta)*buySupport + (alphaYY/beta)*sellSupport

			if finite(compBuy, compSell) {
				putMetric(measurement, "compensator:buy", compBuy, data.UnitCount, data.TimescaleInstantaneous)
				putMetric(measurement, "compensator:sell", compSell, data.UnitCount, data.TimescaleInstantaneous)

				priorCountBuy := float64(len(buyArrivals))
				priorCountSell := float64(len(sellArrivals))
				innoBuy := priorCountBuy - compBuy
				innoSell := priorCountSell - compSell

				putMetric(measurement, "count_innovation:buy", innoBuy, data.UnitCount, data.TimescaleInstantaneous)
				putMetric(measurement, "count_innovation:sell", innoSell, data.UnitCount, data.TimescaleInstantaneous)

				if compBuy > 0 {
					putMetric(measurement, "standardized_innovation:buy", innoBuy/math.Sqrt(compBuy), data.UnitDimensionless, data.TimescaleInstantaneous)
				}
				if compSell > 0 {
					putMetric(measurement, "standardized_innovation:sell", innoSell/math.Sqrt(compSell), data.UnitDimensionless, data.TimescaleInstantaneous)
				}

				// SNR
				snrSum := 0.0
				snrSides := 0
				if compBuy > 0 {
					excess := compBuy - muX*spanPrior
					snrSum += (excess * excess) / compBuy
					snrSides++
				}
				if compSell > 0 {
					excess := compSell - muY*spanPrior
					snrSum += (excess * excess) / compSell
					snrSides++
				}
				if snrSides > 0 {
					snr := snrSum / float64(snrSides)
					putMetric(measurement, "snr", snr, data.UnitDimensionless, data.TimescaleInstantaneous)
					measurement.SNR = snr
					measurement.SNRDefined = true
				}
			}
		}
	}

	if e.modelReady && e.modelSupport > 1 {
		measurement.Maturity = 1.0 - 1.0/e.modelSupport
	} else {
		measurement.Maturity = 0.0
	}

	// 2. Incorporate current event into history
	if len(e.samples) >= MaxArrivalSamples {
		e.samples = e.samples[1:]
	}
	e.samples = append(e.samples, arrivalSample{
		timestamp: at,
		atSec:     atSec,
		mark:      mark,
	})

	// 3. Conditional refit
	e.tryRefit(at, atSec)

	return from, nil
}

func (e *Engine) tryRefit(at time.Time, atSec float64) {
	if len(e.samples) >= 2 && e.samples[len(e.samples)-1].timestamp.Equal(e.samples[len(e.samples)-2].timestamp) {
		return
	}

	buyArrivals := make([]float64, 0, len(e.samples))
	sellArrivals := make([]float64, 0, len(e.samples))
	for _, s := range e.samples {
		if s.mark > 0 {
			buyArrivals = append(buyArrivals, s.atSec)
		} else {
			sellArrivals = append(sellArrivals, s.atSec)
		}
	}

	stream := newArrivalStream(buyArrivals, sellArrivals)
	context, ok := newFitContext(stream, atSec)
	if !ok || !context.enoughEvents(stream) {
		return
	}

	e.eventsSinceFit++
	if e.modelReady && e.eventsSinceFit < context.minFitEvents {
		return
	}

	prior := bivariateFit{}
	if e.modelReady {
		prior = e.model
	}

	estimator := newBivariateEstimator(prior)
	fitted := estimator.fit(stream, atSec)
	if !fitted.valid() {
		return
	}

	e.model = fitted
	e.modelReady = true
	e.modelSupport = float64(context.totalEvents)
	e.eventsSinceFit = 0

	selfOnly := estimator.fitSelfOnly(stream, atSec)
	if selfOnly.valid() {
		e.selfOnlyModel = selfOnly
		e.selfOnlyReady = true
	}
}

func currentWindowStream(buy, sell []float64, horizonSec, mark float64) arrivalStream {
	if mark > 0 {
		return newArrivalStream(append(sortedCopy(buy), horizonSec), sortedCopy(sell))
	}
	return newArrivalStream(sortedCopy(buy), append(sortedCopy(sell), horizonSec))
}

func putMetric(m *data.Measurement[float64], label string, raw float64, unit data.Unit, timescale data.Timescale) {
	m.PutMetric(data.Metric[float64]{
		Label:     label,
		Raw:       raw,
		Unit:      unit,
		Timescale: timescale,
	})
}
