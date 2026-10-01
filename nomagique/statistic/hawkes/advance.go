package hawkes

import (
	"errors"
	"fmt"
	"iter"
	"math"
	"strconv"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

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
	err     error
	history *paths
}

/*
NewCounts creates the empirical arrival stage over the shared registry.
*/
func NewCounts(history *paths) core.Primitive {
	return &Counts{history: history}
}

func (op *Counts) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			p := op.history.at(m.Label)

			side, _ := m.GetProvenance("side")
			mark := -1.0

			if side == "buy" {
				mark = 1.0
			}

			if p.hasLast && m.At.Before(p.lastAt) {
				m.Err = fmt.Errorf("%w: hawkes: regressing event time", core.ErrDomain)

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

			atSec := float64(m.At.UnixNano()) * 1e-9
			fromSec := float64(from.UnixNano()) * 1e-9
			span := atSec - fromSec

			m.From = from

			m.WriteMetric("event_count", count)
			m.WriteMetric("event_count:buy", countBuy)
			m.WriteMetric("event_count:sell", countSell)
			m.WriteMetric("event_fraction:buy", countBuy/count)
			m.WriteMetric("event_fraction:sell", countSell/count)

			if span > 0 {
				m.WriteMetric("arrival_rate:buy", countBuy/span)
				m.WriteMetric("arrival_rate:sell", countSell/span)
				m.WriteMetric("arrival_rate", (countBuy+countSell)/span)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Counts) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
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
	err     error
	history *paths
}

/*
NewExcitation creates the model-evaluation stage over the shared registry.
*/
func NewExcitation(history *paths) core.Primitive {
	return &Excitation{history: history}
}

func (op *Excitation) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)
			p := op.history.at(m.Label)

			m.EnsureMetadata()

			m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(p.support(), 'f', -1, 64))
			delete(m.Metadata, data.MetadataDivergence)
			delete(m.Metadata, data.MetadataNoiseVariance)

			if p.hasSNR {
				m.SetMetadata(data.MetadataDivergence, strconv.FormatFloat(p.divergence(), 'f', -1, 64))
				m.SetMetadata(data.MetadataNoiseVariance, "1")
			}

			if m.Err != nil || !p.modelReady {
				if !yield(arriving) {
					return
				}

				continue
			}

			buyArrivals, sellArrivals := p.sides()

			side, _ := m.GetProvenance("side")
			mark := -1.0

			if side == "buy" {
				mark = 1.0
			}

			atSec := float64(m.At.UnixNano()) * 1e-9
			span := atSec - float64(p.origin().UnixNano())*1e-9

			op.evaluate(m, p, buyArrivals, sellArrivals, atSec, span, mark)

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
func (op *Excitation) evaluate(
	m *data.Measurement[float64],
	p *path,
	buyArrivals, sellArrivals []float64,
	atSec, span, mark float64,
) {
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

	m.WriteMetric("conditional_intensity:buy", lambdaBuy)
	m.WriteMetric("conditional_intensity:sell", lambdaSell)
	m.WriteMetric("conditional_intensity", lambdaBuy+lambdaSell)
	m.WriteMetric("background_rate:buy", muX)
	m.WriteMetric("background_rate:sell", muY)
	m.WriteMetric("background_rate", muX+muY)
	m.WriteMetric("excitation_intensity:buy", excessBuy)
	m.WriteMetric("excitation_intensity:sell", excessSell)

	if lambdaBuy > 0 {
		m.WriteMetric("excitation_fraction:buy", excessBuy/lambdaBuy)
	}

	if lambdaSell > 0 {
		m.WriteMetric("excitation_fraction:sell", excessSell/lambdaSell)
	}

	m.WriteMetric("excitation_amplitude:buy_from_buy", alphaXX)
	m.WriteMetric("excitation_amplitude:buy_from_sell", alphaXY)
	m.WriteMetric("excitation_amplitude:sell_from_buy", alphaYX)
	m.WriteMetric("excitation_amplitude:sell_from_sell", alphaYY)

	if beta > 0 {
		timescale := 1.0 / beta

		m.WriteMetric("excitation_decay", beta)
		m.WriteMetric("excitation_decay:buy_from_buy", beta)
		m.WriteMetric("excitation_decay:buy_from_sell", beta)
		m.WriteMetric("excitation_decay:sell_from_buy", beta)
		m.WriteMetric("excitation_decay:sell_from_sell", beta)
		m.WriteMetric("excitation_timescale", timescale)
		m.WriteMetric("excitation_timescale:buy_from_buy", timescale)
		m.WriteMetric("excitation_timescale:buy_from_sell", timescale)
		m.WriteMetric("excitation_timescale:sell_from_buy", timescale)
		m.WriteMetric("excitation_timescale:sell_from_sell", timescale)
	}

	matrix := branchingMatrix(alphaXX, alphaXY, alphaYX, alphaYY, beta)

	m.WriteMetric("offspring:buy_from_buy", matrix[0][0])
	m.WriteMetric("offspring:buy_from_sell", matrix[0][1])
	m.WriteMetric("offspring:sell_from_buy", matrix[1][0])
	m.WriteMetric("offspring:sell_from_sell", matrix[1][1])
	m.WriteMetric("branching_spectral_radius", spectralRadius(matrix))

	buyParent, sellParent, hasDesc := totalDescendants(alphaXX, alphaXY, alphaYX, alphaYY, beta)

	if hasDesc {
		m.WriteMetric("expected_descendants_from_buy", buyParent)
		m.WriteMetric("expected_descendants_from_sell", sellParent)
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
		m.WriteMetric("log_likelihood:hawkes", hawkesLL)
		m.WriteMetric("log_likelihood_per_event:hawkes", hawkesLL/markedCount)
	}

	poisson := bivariateFit{muX: muX, muY: muY, beta: beta}
	poissonLL, poissonOK := poisson.logLikelihood(streamWindow, atSec)

	if poissonOK {
		m.WriteMetric("log_likelihood:poisson", poissonLL)
	}

	if hawkesOK && poissonOK {
		gainPoisson := hawkesLL - poissonLL
		m.WriteMetric("log_likelihood_gain_vs_poisson", gainPoisson)
		m.WriteMetric("log_likelihood_gain_per_event_vs_poisson", gainPoisson/markedCount)
	}

	if hawkesOK && p.selfOnlyReady {
		selfLL, selfOK := p.selfOnlyModel.logLikelihood(streamWindow, atSec)

		if selfOK {
			gainSelf := hawkesLL - selfLL
			m.WriteMetric("log_likelihood:self_only", selfLL)
			m.WriteMetric("log_likelihood_gain_vs_self_only", gainSelf)
			m.WriteMetric("log_likelihood_gain_per_event_vs_self_only", gainSelf/markedCount)
		}
	}

	buySupport, sellSupport := streamPrior.kernelIntegralSupport(atSec, beta)
	compBuy := muX*spanPrior + (alphaXX/beta)*buySupport + (alphaXY/beta)*sellSupport
	compSell := muY*spanPrior + (alphaYX/beta)*buySupport + (alphaYY/beta)*sellSupport

	priorCountBuy := float64(len(buyArrivals))
	priorCountSell := float64(len(sellArrivals))
	innoBuy := priorCountBuy - compBuy
	innoSell := priorCountSell - compSell

	m.WriteMetric("compensator:buy", compBuy)
	m.WriteMetric("compensator:sell", compSell)
	m.WriteMetric("count_innovation:buy", innoBuy)
	m.WriteMetric("count_innovation:sell", innoSell)

	if compBuy > 0 {
		m.WriteMetric("standardized_innovation:buy", innoBuy/math.Sqrt(compBuy))
	}

	if compSell > 0 {
		m.WriteMetric("standardized_innovation:sell", innoSell/math.Sqrt(compSell))
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

	m.WriteMetric("excitation_mass:buy", excessBuyMass)
	m.WriteMetric("excitation_mass:sell", excessSellMass)

	if compBuy > 0 {
		m.WriteMetric("excitation_share:buy", excessBuyMass/compBuy)
	}

	if compSell > 0 {
		m.WriteMetric("excitation_share:sell", excessSellMass/compSell)
	}

	if compTotal := compBuy + compSell; compTotal > 0 {
		m.WriteMetric("excitation_share", (excessBuyMass+excessSellMass)/compTotal)
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

		m.WriteMetric("snr", p.snr)
	}
}

func (op *Excitation) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
Refit folds the accepted arrival into the retained history and re-estimates
the model from it. The re-estimation only takes effect for the next arrival:
this event was already measured against the model that existed before it.
*/
type Refit struct {
	err     error
	history *paths
}

/*
NewRefit creates the history-advance stage over the shared registry.
*/
func NewRefit(history *paths) core.Primitive {
	return &Refit{history: history}
}

func (op *Refit) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)
			p := op.history.at(m.Label)

			if m.Err == nil {
				side, _ := m.GetProvenance("side")
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

func (op *Refit) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
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
