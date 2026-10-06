package hawkes

import (
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
reject marks a measurement as failed for later pipeline stages. Measurement.err
is unexported under the WORM API, so hawkes tracks rejection out-of-band.
*/
var rejectedMeasurements = map[*data.Measurement]error{}

func reject(m *data.Measurement, err error) {
	if m == nil || err == nil {
		return
	}

	rejectedMeasurements[m] = err
}

func rejected(m *data.Measurement) bool {
	_, ok := rejectedMeasurements[m]
	return ok
}

/*
Paths creates the shared per-symbol arrival registry the pipeline's stages
are wired against. The composition root constructs it once and hands the
same registry to every stage.
*/
func Paths() *paths {
	return newPaths()
}

/*
Counts admits the arrival into the symbol's observation window: it rejects a
regressing event time, then writes the empirical counts, fractions, and
arrival rates the window supports, naming the window's start on the
measurement. Every arrival is yielded exactly once, rejected or not; a
rejected arrival leaves the history untouched.
*/
type Counts struct {
	*core.PrimitiveError
	history *paths
}

/*
NewCounts creates the empirical arrival stage over the shared registry.
*/
func NewCounts(history *paths) core.Primitive {
	return &Counts{
		PrimitiveError: core.NewPrimitiveError(),
		history:        history,
	}
}

func (op *Counts) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement)(arriving)

			if rejected(m) {
				if !yield(arriving) {
					return
				}

				continue
			}

			p := op.history.at(m.Label)

			side := m.Meta("side")
			mark := -1.0

			if side == "buy" {
				mark = 1.0
			}

			if p.hasLast && m.At.Before(p.lastAt) {
				reject(m, fmt.Errorf("%w: hawkes: regressing event time", core.ErrDomain))

				if !yield(arriving) {
					return
				}

				continue
			}

			p.lastAt = m.At
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

			count := countBuy + countSell
			from := m.At

			if len(p.samples) > 0 {
				from = p.origin()
			}

			span := m.At.Sub(from).Seconds()

			if !from.After(m.At) {
				m.From = from
			}

			var metrics []data.Metric

			metrics = append(metrics, data.NewMetric("event_count", count, data.UnitDimensionless, data.TimescaleInstantaneous))
			metrics = append(metrics, data.NewMetric("event_count:buy", countBuy, data.UnitDimensionless, data.TimescaleInstantaneous))
			metrics = append(metrics, data.NewMetric("event_count:sell", countSell, data.UnitDimensionless, data.TimescaleInstantaneous))
			metrics = append(metrics, data.NewMetric("event_fraction:buy", countBuy/count, data.UnitDimensionless, data.TimescaleInstantaneous))
			metrics = append(metrics, data.NewMetric("event_fraction:sell", countSell/count, data.UnitDimensionless, data.TimescaleInstantaneous))

			if span > 0 {
				metrics = append(metrics, data.NewMetric("arrival_rate:buy", countBuy/span, data.UnitRate, data.TimescaleInstantaneous))
				metrics = append(metrics, data.NewMetric("arrival_rate:sell", countSell/span, data.UnitRate, data.TimescaleInstantaneous))
				metrics = append(metrics, data.NewMetric("arrival_rate", (countBuy+countSell)/span, data.UnitRate, data.TimescaleInstantaneous))
			}

			if len(metrics) > 0 {
				m.Write(metrics...)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

/*
Excitation measures the arrival against the model fitted strictly before it:
conditional intensities, excitation decomposition, the branching matrix and
its spectral facts, in-window likelihoods against the nested Poisson and
self-only restrictions, and the compensator innovations whose excitation
share carries the clustering the fit measured. Without a fitted model there
is nothing to measure against and the measurement moves through untouched.
*/
type Excitation struct {
	*core.PrimitiveError
	history *paths
}

/*
NewExcitation creates the model-evaluation stage over the shared registry.
*/
func NewExcitation(history *paths) core.Primitive {
	return &Excitation{
		PrimitiveError: core.NewPrimitiveError(),
		history:        history,
	}
}

func (op *Excitation) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement)(arriving)
			p := op.history.at(m.Label)

			if rejected(m) || !p.modelReady {
				if !yield(arriving) {
					return
				}

				continue
			}

			buyArrivals, sellArrivals := p.sides()

			side := m.Meta("side")
			mark := -1.0

			if side == "buy" {
				mark = 1.0
			}

			atSec := float64(m.At.UnixNano()) * 1e-9
			span := m.At.Sub(p.origin()).Seconds()

			var _ float64 = span
			op.evaluate(m, p, buyArrivals, sellArrivals, atSec, mark)

			if !yield(arriving) {
				return
			}
		}
	}
}

/*
evaluate publishes one event's model-conditioned facts, exactly the
mathematics the fitted bivariate process defines: pre-arrival intensities,
excitation decomposition, branching descent, likelihoods against nested
restrictions, and compensator innovations.
*/
func (op *Excitation) evaluate(m *data.Measurement, p *path, buyArrivals, sellArrivals []float64, atSec, mark float64) {
	model := p.model

	muX := model.muX
	muY := model.muY
	alphaXX := model.alphaXX
	alphaXY := model.alphaXY
	alphaYX := model.alphaYX
	alphaYY := model.alphaYY
	beta := model.beta

	lambdaBuy := intensityAt(buyArrivals, sellArrivals, atSec, muX, alphaXX, alphaXY, beta)
	lambdaSell := intensityAt(buyArrivals, sellArrivals, atSec, muY, alphaYX, alphaYY, beta)

	excessBuy := lambdaBuy - muX
	excessSell := lambdaSell - muY

	var metrics []data.Metric

	metrics = append(metrics, data.NewMetric("conditional_intensity:buy", lambdaBuy, data.UnitRate, data.TimescaleInstantaneous))
	metrics = append(metrics, data.NewMetric("conditional_intensity:sell", lambdaSell, data.UnitRate, data.TimescaleInstantaneous))
	metrics = append(metrics, data.NewMetric("conditional_intensity", lambdaBuy+lambdaSell, data.UnitRate, data.TimescaleInstantaneous))
	metrics = append(metrics, data.NewMetric("background_rate:buy", muX, data.UnitRate, data.TimescaleInstantaneous))
	metrics = append(metrics, data.NewMetric("background_rate:sell", muY, data.UnitRate, data.TimescaleInstantaneous))
	metrics = append(metrics, data.NewMetric("background_rate", muX+muY, data.UnitRate, data.TimescaleInstantaneous))
	metrics = append(metrics, data.NewMetric("excitation_intensity:buy", excessBuy, data.UnitRate, data.TimescaleInstantaneous))
	metrics = append(metrics, data.NewMetric("excitation_intensity:sell", excessSell, data.UnitRate, data.TimescaleInstantaneous))

	if lambdaBuy > 0 {
		metrics = append(metrics, data.NewMetric("excitation_fraction:buy", excessBuy/lambdaBuy, data.UnitDimensionless, data.TimescaleInstantaneous))
	}

	if lambdaSell > 0 {
		metrics = append(metrics, data.NewMetric("excitation_fraction:sell", excessSell/lambdaSell, data.UnitDimensionless, data.TimescaleInstantaneous))
	}

	metrics = append(metrics, data.NewMetric("excitation_amplitude:buy_from_buy", alphaXX, data.UnitRate, data.TimescaleInstantaneous))
	metrics = append(metrics, data.NewMetric("excitation_amplitude:buy_from_sell", alphaXY, data.UnitRate, data.TimescaleInstantaneous))
	metrics = append(metrics, data.NewMetric("excitation_amplitude:sell_from_buy", alphaYX, data.UnitRate, data.TimescaleInstantaneous))
	metrics = append(metrics, data.NewMetric("excitation_amplitude:sell_from_sell", alphaYY, data.UnitRate, data.TimescaleInstantaneous))

	if beta > 0 {
		timescale := 1.0 / beta

		metrics = append(metrics, data.NewMetric("excitation_decay", beta, data.UnitRate, data.TimescaleInstantaneous))
		metrics = append(metrics, data.NewMetric("excitation_decay:buy_from_buy", beta, data.UnitRate, data.TimescaleInstantaneous))
		metrics = append(metrics, data.NewMetric("excitation_decay:buy_from_sell", beta, data.UnitRate, data.TimescaleInstantaneous))
		metrics = append(metrics, data.NewMetric("excitation_decay:sell_from_buy", beta, data.UnitRate, data.TimescaleInstantaneous))
		metrics = append(metrics, data.NewMetric("excitation_decay:sell_from_sell", beta, data.UnitRate, data.TimescaleInstantaneous))
		metrics = append(metrics, data.NewMetric("excitation_timescale", timescale, data.UnitDuration, data.TimescaleInstantaneous))
		metrics = append(metrics, data.NewMetric("excitation_timescale:buy_from_buy", timescale, data.UnitDuration, data.TimescaleInstantaneous))
		metrics = append(metrics, data.NewMetric("excitation_timescale:buy_from_sell", timescale, data.UnitDuration, data.TimescaleInstantaneous))
		metrics = append(metrics, data.NewMetric("excitation_timescale:sell_from_buy", timescale, data.UnitDuration, data.TimescaleInstantaneous))
		metrics = append(metrics, data.NewMetric("excitation_timescale:sell_from_sell", timescale, data.UnitDuration, data.TimescaleInstantaneous))
	}

	matrix := branchingMatrix(alphaXX, alphaXY, alphaYX, alphaYY, beta)

	metrics = append(metrics, data.NewMetric("offspring:buy_from_buy", matrix[0][0], data.UnitDimensionless, data.TimescaleInstantaneous))
	metrics = append(metrics, data.NewMetric("offspring:buy_from_sell", matrix[0][1], data.UnitDimensionless, data.TimescaleInstantaneous))
	metrics = append(metrics, data.NewMetric("offspring:sell_from_buy", matrix[1][0], data.UnitDimensionless, data.TimescaleInstantaneous))
	metrics = append(metrics, data.NewMetric("offspring:sell_from_sell", matrix[1][1], data.UnitDimensionless, data.TimescaleInstantaneous))
	metrics = append(metrics, data.NewMetric("branching_spectral_radius", spectralRadius(matrix), data.UnitDimensionless, data.TimescaleInstantaneous))

	buyParent, sellParent, hasDesc := totalDescendants(alphaXX, alphaXY, alphaYX, alphaYY, beta)

	if hasDesc {
		metrics = append(metrics, data.NewMetric("expected_descendants_from_buy", buyParent, data.UnitDimensionless, data.TimescaleInstantaneous))
		metrics = append(metrics, data.NewMetric("expected_descendants_from_sell", sellParent, data.UnitDimensionless, data.TimescaleInstantaneous))
	}

	streamPrior := newArrivalStream(buyArrivals, sellArrivals)
	spanPrior := streamPrior.span(atSec)

	if spanPrior <= 0 {
		return
	}

	streamWindow := currentWindowStream(buyArrivals, sellArrivals, atSec, mark)
	markedCount := float64(len(streamWindow.marked))

	hawkesLL, hawkesOK := model.logLikelihood(streamWindow, atSec)

	if hawkesOK {
		metrics = append(metrics, data.NewMetric("log_likelihood:hawkes", hawkesLL, data.UnitDimensionless, data.TimescaleInstantaneous))
		metrics = append(metrics, data.NewMetric("log_likelihood_per_event:hawkes", hawkesLL/markedCount, data.UnitDimensionless, data.TimescaleInstantaneous))
	}

	poisson := bivariateFit{muX: muX, muY: muY, beta: beta}
	poissonLL, poissonOK := poisson.logLikelihood(streamWindow, atSec)

	if poissonOK {
		metrics = append(metrics, data.NewMetric("log_likelihood:poisson", poissonLL, data.UnitDimensionless, data.TimescaleInstantaneous))
	}

	if hawkesOK && poissonOK {
		gainPoisson := hawkesLL - poissonLL
		metrics = append(metrics, data.NewMetric("log_likelihood_gain_vs_poisson", gainPoisson, data.UnitDimensionless, data.TimescaleInstantaneous))
		metrics = append(metrics, data.NewMetric("log_likelihood_gain_per_event_vs_poisson", gainPoisson/markedCount, data.UnitDimensionless, data.TimescaleInstantaneous))
	}

	if hawkesOK && p.selfOnlyReady {
		selfLL, selfOK := p.selfOnlyModel.logLikelihood(streamWindow, atSec)

		if selfOK {
			gainSelf := hawkesLL - selfLL
			metrics = append(metrics, data.NewMetric("log_likelihood:self_only", selfLL, data.UnitDimensionless, data.TimescaleInstantaneous))
			metrics = append(metrics, data.NewMetric("log_likelihood_gain_vs_self_only", gainSelf, data.UnitDimensionless, data.TimescaleInstantaneous))
			metrics = append(metrics, data.NewMetric("log_likelihood_gain_per_event_vs_self_only", gainSelf/markedCount, data.UnitDimensionless, data.TimescaleInstantaneous))
		}
	}

	buySupport, sellSupport := streamPrior.kernelIntegralSupport(atSec, beta)
	compBuy := muX*spanPrior + (alphaXX/beta)*buySupport + (alphaXY/beta)*sellSupport
	compSell := muY*spanPrior + (alphaYX/beta)*buySupport + (alphaYY/beta)*sellSupport

	priorCountBuy := float64(len(buyArrivals))
	priorCountSell := float64(len(sellArrivals))
	innoBuy := priorCountBuy - compBuy
	innoSell := priorCountSell - compSell

	metrics = append(metrics, data.NewMetric("compensator:buy", compBuy, data.UnitDimensionless, data.TimescaleInstantaneous))
	metrics = append(metrics, data.NewMetric("compensator:sell", compSell, data.UnitDimensionless, data.TimescaleInstantaneous))
	metrics = append(metrics, data.NewMetric("count_innovation:buy", innoBuy, data.UnitDimensionless, data.TimescaleInstantaneous))
	metrics = append(metrics, data.NewMetric("count_innovation:sell", innoSell, data.UnitDimensionless, data.TimescaleInstantaneous))

	if compBuy > 0 {
		metrics = append(metrics, data.NewMetric("standardized_innovation:buy", innoBuy/math.Sqrt(compBuy), data.UnitDimensionless, data.TimescaleInstantaneous))
	}

	if compSell > 0 {
		metrics = append(metrics, data.NewMetric("standardized_innovation:sell", innoSell/math.Sqrt(compSell), data.UnitDimensionless, data.TimescaleInstantaneous))
	}

	// excitation_share is the excitation's share of the integrated
	// intensity over the whole observation span, where excitation_fraction
	// above is that share at this one instant. The two answer different
	// questions: on a bursty stream the instantaneous form samples a
	// decaying exponential at whatever moment a frame happens to land, so
	// it reads near zero between bursts even when the fit has found strong
	// clustering. The integrated form carries the clustering the fit
	// actually measured, and is what a consumer ranking self-excitation
	// across symbols must read.
	excessBuyMass := compBuy - muX*spanPrior
	excessSellMass := compSell - muY*spanPrior

	metrics = append(metrics, data.NewMetric("excitation_mass:buy", excessBuyMass, data.UnitDimensionless, data.TimescaleInstantaneous))
	metrics = append(metrics, data.NewMetric("excitation_mass:sell", excessSellMass, data.UnitDimensionless, data.TimescaleInstantaneous))

	if compBuy > 0 {
		metrics = append(metrics, data.NewMetric("excitation_share:buy", excessBuyMass/compBuy, data.UnitDimensionless, data.TimescaleInstantaneous))
	}

	if compSell > 0 {
		metrics = append(metrics, data.NewMetric("excitation_share:sell", excessSellMass/compSell, data.UnitDimensionless, data.TimescaleInstantaneous))
	}

	if compTotal := compBuy + compSell; compTotal > 0 {
		metrics = append(metrics, data.NewMetric("excitation_share", (excessBuyMass+excessSellMass)/compTotal, data.UnitDimensionless, data.TimescaleInstantaneous))
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

		metrics = append(metrics, data.NewMetric("snr", p.snr, data.UnitDimensionless, data.TimescaleInstantaneous))
	}

	if len(metrics) > 0 {
		if m.ID != 0 {
			next := data.NewMeasurement(m.Epoch, m.Label, m.Source, m.SeqIdx, m.Tick)
			next.At = m.At
			next.From = m.From
			next.Write(metrics...)
			*m = *next
		} else {
			m.Write(metrics...)
		}
	}
}

/*
Refit folds the accepted arrival into the retained history and re-estimates
the model from it. The re-estimation only takes effect for the next arrival:
this event was already measured against the model that existed before it.
*/
type Refit struct {
	*core.PrimitiveError
	history *paths
}

/*
NewRefit creates the history-advance stage over the shared registry.
*/
func NewRefit(history *paths) core.Primitive {
	return &Refit{
		PrimitiveError: core.NewPrimitiveError(),
		history:        history,
	}
}

func (op *Refit) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement)(arriving)
			p := op.history.at(m.Label)

			if !rejected(m) {
				side := m.Meta("side")
				mark := -1.0

				if side == "buy" {
					mark = 1.0
				}

				atSec := float64(m.At.UnixNano()) * 1e-9

				p.remember(m.At, atSec, mark)
				p.refit(atSec)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (p *path) support() float64 {
	if !p.modelReady {
		return 0
	}

	return p.modelSupport
}

func (p *path) divergence() float64 {
	if !p.hasSNR {
		return 0
	}

	return math.Sqrt(p.snr)
}

/*
currentWindowStream appends the current event to its side's history so the
likelihood window includes the event being measured.
*/
func currentWindowStream(buy, sell []float64, horizonSec, mark float64) arrivalStream {
	if mark > 0 {
		return newArrivalStream(append(sortedCopy(buy), horizonSec), sortedCopy(sell))
	}

	return newArrivalStream(sortedCopy(buy), append(sortedCopy(sell), horizonSec))
}
