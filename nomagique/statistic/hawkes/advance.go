package hawkes

import (
	"errors"
	"fmt"
	"iter"
	"math"
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
			p := op.history.at(m.Label)

			side := m.Provenance["side"]
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

			m.Metrics["event_count"] = m.Metrics["event_count"].Write(count)
			m.Metrics["event_count:buy"] = m.Metrics["event_count:buy"].Write(countBuy)
			m.Metrics["event_count:sell"] = m.Metrics["event_count:sell"].Write(countSell)
			m.Metrics["event_fraction:buy"] = m.Metrics["event_fraction:buy"].Write(countBuy / count)
			m.Metrics["event_fraction:sell"] = m.Metrics["event_fraction:sell"].Write(countSell / count)

			if span > 0 {
				m.Metrics["arrival_rate:buy"] = m.Metrics["arrival_rate:buy"].Write(countBuy / span)
				m.Metrics["arrival_rate:sell"] = m.Metrics["arrival_rate:sell"].Write(countSell / span)
				m.Metrics["arrival_rate"] = m.Metrics["arrival_rate"].Write((countBuy + countSell) / span)
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

			metadata := map[string]float64{data.MetadataSupport: p.support()}

			if p.hasSNR {
				metadata[data.MetadataDivergence] = p.divergence()
				metadata[data.MetadataNoiseVariance] = 1.0
			}

			m.Metadata = metadata

			if m.Err != nil || !p.modelReady {
				if !yield(arriving) {
					return
				}

				continue
			}

			buyArrivals, sellArrivals := p.sides()

			side := m.Provenance["side"]
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

	m.Metrics["conditional_intensity:buy"] = m.Metrics["conditional_intensity:buy"].Write(lambdaBuy)
	m.Metrics["conditional_intensity:sell"] = m.Metrics["conditional_intensity:sell"].Write(lambdaSell)
	m.Metrics["conditional_intensity"] = m.Metrics["conditional_intensity"].Write(lambdaBuy + lambdaSell)
	m.Metrics["background_rate:buy"] = m.Metrics["background_rate:buy"].Write(muX)
	m.Metrics["background_rate:sell"] = m.Metrics["background_rate:sell"].Write(muY)
	m.Metrics["background_rate"] = m.Metrics["background_rate"].Write(muX + muY)
	m.Metrics["excitation_intensity:buy"] = m.Metrics["excitation_intensity:buy"].Write(excessBuy)
	m.Metrics["excitation_intensity:sell"] = m.Metrics["excitation_intensity:sell"].Write(excessSell)

	if lambdaBuy > 0 {
		m.Metrics["excitation_fraction:buy"] = m.Metrics["excitation_fraction:buy"].Write(excessBuy / lambdaBuy)
	}

	if lambdaSell > 0 {
		m.Metrics["excitation_fraction:sell"] = m.Metrics["excitation_fraction:sell"].Write(excessSell / lambdaSell)
	}

	m.Metrics["excitation_amplitude:buy_from_buy"] = m.Metrics["excitation_amplitude:buy_from_buy"].Write(alphaXX)
	m.Metrics["excitation_amplitude:buy_from_sell"] = m.Metrics["excitation_amplitude:buy_from_sell"].Write(alphaXY)
	m.Metrics["excitation_amplitude:sell_from_buy"] = m.Metrics["excitation_amplitude:sell_from_buy"].Write(alphaYX)
	m.Metrics["excitation_amplitude:sell_from_sell"] = m.Metrics["excitation_amplitude:sell_from_sell"].Write(alphaYY)

	if beta > 0 {
		timescale := 1.0 / beta

		m.Metrics["excitation_decay"] = m.Metrics["excitation_decay"].Write(beta)
		m.Metrics["excitation_decay:buy_from_buy"] = m.Metrics["excitation_decay:buy_from_buy"].Write(beta)
		m.Metrics["excitation_decay:buy_from_sell"] = m.Metrics["excitation_decay:buy_from_sell"].Write(beta)
		m.Metrics["excitation_decay:sell_from_buy"] = m.Metrics["excitation_decay:sell_from_buy"].Write(beta)
		m.Metrics["excitation_decay:sell_from_sell"] = m.Metrics["excitation_decay:sell_from_sell"].Write(beta)
		m.Metrics["excitation_timescale"] = m.Metrics["excitation_timescale"].Write(timescale)
		m.Metrics["excitation_timescale:buy_from_buy"] = m.Metrics["excitation_timescale:buy_from_buy"].Write(timescale)
		m.Metrics["excitation_timescale:buy_from_sell"] = m.Metrics["excitation_timescale:buy_from_sell"].Write(timescale)
		m.Metrics["excitation_timescale:sell_from_buy"] = m.Metrics["excitation_timescale:sell_from_buy"].Write(timescale)
		m.Metrics["excitation_timescale:sell_from_sell"] = m.Metrics["excitation_timescale:sell_from_sell"].Write(timescale)
	}

	matrix := branchingMatrix(alphaXX, alphaXY, alphaYX, alphaYY, beta)

	m.Metrics["offspring:buy_from_buy"] = m.Metrics["offspring:buy_from_buy"].Write(matrix[0][0])
	m.Metrics["offspring:buy_from_sell"] = m.Metrics["offspring:buy_from_sell"].Write(matrix[0][1])
	m.Metrics["offspring:sell_from_buy"] = m.Metrics["offspring:sell_from_buy"].Write(matrix[1][0])
	m.Metrics["offspring:sell_from_sell"] = m.Metrics["offspring:sell_from_sell"].Write(matrix[1][1])
	m.Metrics["branching_spectral_radius"] = m.Metrics["branching_spectral_radius"].Write(spectralRadius(matrix))

	buyParent, sellParent, hasDesc := totalDescendants(alphaXX, alphaXY, alphaYX, alphaYY, beta)

	if hasDesc {
		m.Metrics["expected_descendants_from_buy"] = m.Metrics["expected_descendants_from_buy"].Write(buyParent)
		m.Metrics["expected_descendants_from_sell"] = m.Metrics["expected_descendants_from_sell"].Write(sellParent)
	}

	streamWindow := currentWindowStream(buyArrivals, sellArrivals, atSec, mark)
	hawkesLL := model.logLikelihood(streamWindow, atSec)
	poisson := bivariateFit{muX: muX, muY: muY, beta: beta}
	poissonLL := poisson.logLikelihood(streamWindow, atSec)

	if len(streamWindow.marked) > 0 {
		markedCount := float64(len(streamWindow.marked))

		m.Metrics["log_likelihood:hawkes"] = m.Metrics["log_likelihood:hawkes"].Write(hawkesLL)
		m.Metrics["log_likelihood:poisson"] = m.Metrics["log_likelihood:poisson"].Write(poissonLL)
		m.Metrics["log_likelihood_per_event:hawkes"] = m.Metrics["log_likelihood_per_event:hawkes"].Write(hawkesLL / markedCount)
		m.Metrics["log_likelihood_gain_vs_poisson"] = m.Metrics["log_likelihood_gain_vs_poisson"].Write(hawkesLL - poissonLL)
		m.Metrics["log_likelihood_gain_per_event_vs_poisson"] = m.Metrics["log_likelihood_gain_per_event_vs_poisson"].Write((hawkesLL - poissonLL) / markedCount)

		if p.selfOnlyReady {
			selfLL := p.selfOnlyModel.logLikelihood(streamWindow, atSec)

			m.Metrics["log_likelihood:self_only"] = m.Metrics["log_likelihood:self_only"].Write(selfLL)
			m.Metrics["log_likelihood_gain_vs_self_only"] = m.Metrics["log_likelihood_gain_vs_self_only"].Write(hawkesLL - selfLL)
			m.Metrics["log_likelihood_gain_per_event_vs_self_only"] = m.Metrics["log_likelihood_gain_per_event_vs_self_only"].Write((hawkesLL - selfLL) / markedCount)
		}
	}

	streamPrior := newArrivalStream(buyArrivals, sellArrivals)
	spanPrior := streamPrior.span(atSec)

	if spanPrior <= 0 {
		return
	}

	buySupport, sellSupport := streamPrior.kernelIntegralSupport(atSec, beta)
	compBuy := muX*spanPrior + (alphaXX/beta)*buySupport + (alphaXY/beta)*sellSupport
	compSell := muY*spanPrior + (alphaYX/beta)*buySupport + (alphaYY/beta)*sellSupport

	priorCountBuy := float64(len(buyArrivals))
	priorCountSell := float64(len(sellArrivals))
	innoBuy := priorCountBuy - compBuy
	innoSell := priorCountSell - compSell

	m.Metrics["compensator:buy"] = m.Metrics["compensator:buy"].Write(compBuy)
	m.Metrics["compensator:sell"] = m.Metrics["compensator:sell"].Write(compSell)
	m.Metrics["count_innovation:buy"] = m.Metrics["count_innovation:buy"].Write(innoBuy)
	m.Metrics["count_innovation:sell"] = m.Metrics["count_innovation:sell"].Write(innoSell)

	if compBuy > 0 {
		m.Metrics["standardized_innovation:buy"] = m.Metrics["standardized_innovation:buy"].Write(innoBuy / math.Sqrt(compBuy))
	}

	if compSell > 0 {
		m.Metrics["standardized_innovation:sell"] = m.Metrics["standardized_innovation:sell"].Write(innoSell / math.Sqrt(compSell))
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

	m.Metrics["excitation_mass:buy"] = m.Metrics["excitation_mass:buy"].Write(excessBuyMass)
	m.Metrics["excitation_mass:sell"] = m.Metrics["excitation_mass:sell"].Write(excessSellMass)

	if compBuy > 0 {
		m.Metrics["excitation_share:buy"] = m.Metrics["excitation_share:buy"].Write(excessBuyMass / compBuy)
	}

	if compSell > 0 {
		m.Metrics["excitation_share:sell"] = m.Metrics["excitation_share:sell"].Write(excessSellMass / compSell)
	}

	if compTotal := compBuy + compSell; compTotal > 0 {
		m.Metrics["excitation_share"] = m.Metrics["excitation_share"].Write((excessBuyMass + excessSellMass) / compTotal)
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

		m.Metrics["snr"] = m.Metrics["snr"].Write(p.snr)
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
				side := m.Provenance["side"]
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
