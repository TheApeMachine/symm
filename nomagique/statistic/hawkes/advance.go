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
			m := *(**data.Measurement)(arriving)

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

			span := m.At.Sub(from).Seconds()

			if !from.After(m.At) {
				m.From = from
			}

			m.SetMetric("event_count", data.NewMetric(
				"event_count",
				data.UnitDimensionless,
				data.TimescaleInstantaneous,
				count,
				math.Sqrt(count),
			).Write(count))
			m.SetMetric("event_count:buy", data.NewMetric(
				"event_count:buy",
				data.UnitDimensionless,
				data.TimescaleInstantaneous,
				count/2.0,
				count/2.0,
			).Write(countBuy))
			m.SetMetric("event_count:sell", data.NewMetric(
				"event_count:sell",
				data.UnitDimensionless,
				data.TimescaleInstantaneous,
				count/2.0,
				count/2.0,
			).Write(countSell))
			m.SetMetric("event_fraction:buy", data.NewMetric(
				"event_fraction:buy",
				data.UnitDimensionless,
				data.TimescaleInstantaneous,
				0.5,
				0.5,
			).Write(countBuy/count))
			m.SetMetric("event_fraction:sell", data.NewMetric(
				"event_fraction:sell",
				data.UnitDimensionless,
				data.TimescaleInstantaneous,
				0.5,
				0.5,
			).Write(countSell/count))

			if span > 0 {
				rateScale := math.Sqrt(count) / span
				m.SetMetric("arrival_rate:buy", data.NewMetric(
					"arrival_rate:buy",
					data.UnitRate,
					data.TimescaleInstantaneous,
					(count/2.0)/span,
					rateScale/2.0,
				).Write(countBuy/span))
				m.SetMetric("arrival_rate:sell", data.NewMetric(
					"arrival_rate:sell",
					data.UnitRate,
					data.TimescaleInstantaneous,
					(count/2.0)/span,
					rateScale/2.0,
				).Write(countSell/span))
				m.SetMetric("arrival_rate", data.NewMetric(
					"arrival_rate",
					data.UnitRate,
					data.TimescaleInstantaneous,
					count/span,
					rateScale,
				).Write((countBuy+countSell)/span))
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
			m := *(**data.Measurement)(arriving)
			p := op.history.at(m.Label)

			m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(p.support(), 'f', -1, 64))
			m.DeleteMetadata(data.MetadataDivergence)
			m.DeleteMetadata(data.MetadataNoiseVariance)

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

	m.SetMetric("conditional_intensity:buy", data.NewMetric(
		"conditional_intensity:buy",
		data.UnitRate,
		data.TimescaleInstantaneous,
		muX,
		muX,
	).Write(lambdaBuy))
	m.SetMetric("conditional_intensity:sell", data.NewMetric(
		"conditional_intensity:sell",
		data.UnitRate,
		data.TimescaleInstantaneous,
		muY,
		muY,
	).Write(lambdaSell))
	m.SetMetric("conditional_intensity", data.NewMetric(
		"conditional_intensity",
		data.UnitRate,
		data.TimescaleInstantaneous,
		muX+muY,
		muX+muY,
	).Write(lambdaBuy+lambdaSell))
	m.SetMetric("background_rate:buy", data.NewMetric(
		"background_rate:buy",
		data.UnitRate,
		data.TimescaleInstantaneous,
		muX,
		muX,
	).Write(muX))
	m.SetMetric("background_rate:sell", data.NewMetric(
		"background_rate:sell",
		data.UnitRate,
		data.TimescaleInstantaneous,
		muY,
		muY,
	).Write(muY))
	m.SetMetric("background_rate", data.NewMetric(
		"background_rate",
		data.UnitRate,
		data.TimescaleInstantaneous,
		muX+muY,
		muX+muY,
	).Write(muX+muY))
	m.SetMetric("excitation_intensity:buy", data.NewMetric(
		"excitation_intensity:buy",
		data.UnitRate,
		data.TimescaleInstantaneous,
		0.0,
		muX,
	).Write(excessBuy))
	m.SetMetric("excitation_intensity:sell", data.NewMetric(
		"excitation_intensity:sell",
		data.UnitRate,
		data.TimescaleInstantaneous,
		0.0,
		muY,
	).Write(excessSell))

	if lambdaBuy > 0 {
		m.SetMetric("excitation_fraction:buy", data.NewMetric(
			"excitation_fraction:buy",
			data.UnitDimensionless,
			data.TimescaleInstantaneous,
			0.5,
			0.5,
		).Write(excessBuy/lambdaBuy))
	}

	if lambdaSell > 0 {
		m.SetMetric("excitation_fraction:sell", data.NewMetric(
			"excitation_fraction:sell",
			data.UnitDimensionless,
			data.TimescaleInstantaneous,
			0.5,
			0.5,
		).Write(excessSell/lambdaSell))
	}

	m.SetMetric("excitation_amplitude:buy_from_buy", data.NewMetric(
		"excitation_amplitude:buy_from_buy",
		data.UnitRate,
		data.TimescaleInstantaneous,
		0.0,
		beta,
	).Write(alphaXX))
	m.SetMetric("excitation_amplitude:buy_from_sell", data.NewMetric(
		"excitation_amplitude:buy_from_sell",
		data.UnitRate,
		data.TimescaleInstantaneous,
		0.0,
		beta,
	).Write(alphaXY))
	m.SetMetric("excitation_amplitude:sell_from_buy", data.NewMetric(
		"excitation_amplitude:sell_from_buy",
		data.UnitRate,
		data.TimescaleInstantaneous,
		0.0,
		beta,
	).Write(alphaYX))
	m.SetMetric("excitation_amplitude:sell_from_sell", data.NewMetric(
		"excitation_amplitude:sell_from_sell",
		data.UnitRate,
		data.TimescaleInstantaneous,
		0.0,
		beta,
	).Write(alphaYY))

	if beta > 0 {
		timescale := 1.0 / beta

		m.SetMetric("excitation_decay", data.NewMetric(
			"excitation_decay",
			data.UnitRate,
			data.TimescaleInstantaneous,
			0.0,
			beta,
		).Write(beta))
		m.SetMetric("excitation_decay:buy_from_buy", data.NewMetric(
			"excitation_decay:buy_from_buy",
			data.UnitRate,
			data.TimescaleInstantaneous,
			0.0,
			beta,
		).Write(beta))
		m.SetMetric("excitation_decay:buy_from_sell", data.NewMetric(
			"excitation_decay:buy_from_sell",
			data.UnitRate,
			data.TimescaleInstantaneous,
			0.0,
			beta,
		).Write(beta))
		m.SetMetric("excitation_decay:sell_from_buy", data.NewMetric(
			"excitation_decay:sell_from_buy",
			data.UnitRate,
			data.TimescaleInstantaneous,
			0.0,
			beta,
		).Write(beta))
		m.SetMetric("excitation_decay:sell_from_sell", data.NewMetric(
			"excitation_decay:sell_from_sell",
			data.UnitRate,
			data.TimescaleInstantaneous,
			0.0,
			beta,
		).Write(beta))
		m.SetMetric("excitation_timescale", data.NewMetric(
			"excitation_timescale",
			data.UnitDuration,
			data.TimescaleInstantaneous,
			0.0,
			timescale,
		).Write(timescale))
		m.SetMetric("excitation_timescale:buy_from_buy", data.NewMetric(
			"excitation_timescale:buy_from_buy",
			data.UnitDuration,
			data.TimescaleInstantaneous,
			0.0,
			timescale,
		).Write(timescale))
		m.SetMetric("excitation_timescale:buy_from_sell", data.NewMetric(
			"excitation_timescale:buy_from_sell",
			data.UnitDuration,
			data.TimescaleInstantaneous,
			0.0,
			timescale,
		).Write(timescale))
		m.SetMetric("excitation_timescale:sell_from_buy", data.NewMetric(
			"excitation_timescale:sell_from_buy",
			data.UnitDuration,
			data.TimescaleInstantaneous,
			0.0,
			timescale,
		).Write(timescale))
		m.SetMetric("excitation_timescale:sell_from_sell", data.NewMetric(
			"excitation_timescale:sell_from_sell",
			data.UnitDuration,
			data.TimescaleInstantaneous,
			0.0,
			timescale,
		).Write(timescale))
	}

	matrix := branchingMatrix(alphaXX, alphaXY, alphaYX, alphaYY, beta)

	m.SetMetric("offspring:buy_from_buy", data.NewMetric(
		"offspring:buy_from_buy",
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
		0.0,
		1.0,
	).Write(matrix[0][0]))
	m.SetMetric("offspring:buy_from_sell", data.NewMetric(
		"offspring:buy_from_sell",
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
		0.0,
		1.0,
	).Write(matrix[0][1]))
	m.SetMetric("offspring:sell_from_buy", data.NewMetric(
		"offspring:sell_from_buy",
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
		0.0,
		1.0,
	).Write(matrix[1][0]))
	m.SetMetric("offspring:sell_from_sell", data.NewMetric(
		"offspring:sell_from_sell",
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
		0.0,
		1.0,
	).Write(matrix[1][1]))
	m.SetMetric("branching_spectral_radius", data.NewMetric(
		"branching_spectral_radius",
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
		0.0,
		1.0,
	).Write(spectralRadius(matrix)))

	buyParent, sellParent, hasDesc := totalDescendants(alphaXX, alphaXY, alphaYX, alphaYY, beta)

	if hasDesc {
		m.SetMetric("expected_descendants_from_buy", data.NewMetric(
			"expected_descendants_from_buy",
			data.UnitDimensionless,
			data.TimescaleInstantaneous,
			1.0,
			1.0,
		).Write(buyParent))
		m.SetMetric("expected_descendants_from_sell", data.NewMetric(
			"expected_descendants_from_sell",
			data.UnitDimensionless,
			data.TimescaleInstantaneous,
			1.0,
			1.0,
		).Write(sellParent))
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
		m.SetMetric("log_likelihood:hawkes", data.NewMetric(
			"log_likelihood:hawkes",
			data.UnitDimensionless,
			data.TimescaleInstantaneous,
			0.0,
			markedCount,
		).Write(hawkesLL))
		m.SetMetric("log_likelihood_per_event:hawkes", data.NewMetric(
			"log_likelihood_per_event:hawkes",
			data.UnitDimensionless,
			data.TimescaleInstantaneous,
			0.0,
			1.0,
		).Write(hawkesLL/markedCount))
	}

	poisson := bivariateFit{muX: muX, muY: muY, beta: beta}
	poissonLL, poissonOK := poisson.logLikelihood(streamWindow, atSec)

	if poissonOK {
		m.SetMetric("log_likelihood:poisson", data.NewMetric(
			"log_likelihood:poisson",
			data.UnitDimensionless,
			data.TimescaleInstantaneous,
			0.0,
			markedCount,
		).Write(poissonLL))
	}

	if hawkesOK && poissonOK {
		gainPoisson := hawkesLL - poissonLL
		m.SetMetric("log_likelihood_gain_vs_poisson", data.NewMetric(
			"log_likelihood_gain_vs_poisson",
			data.UnitDimensionless,
			data.TimescaleInstantaneous,
			0.0,
			markedCount,
		).Write(gainPoisson))
		m.SetMetric("log_likelihood_gain_per_event_vs_poisson", data.NewMetric(
			"log_likelihood_gain_per_event_vs_poisson",
			data.UnitDimensionless,
			data.TimescaleInstantaneous,
			0.0,
			1.0,
		).Write(gainPoisson/markedCount))
	}

	if hawkesOK && p.selfOnlyReady {
		selfLL, selfOK := p.selfOnlyModel.logLikelihood(streamWindow, atSec)

		if selfOK {
			gainSelf := hawkesLL - selfLL
			m.SetMetric("log_likelihood:self_only", data.NewMetric(
				"log_likelihood:self_only",
				data.UnitDimensionless,
				data.TimescaleInstantaneous,
				0.0,
				markedCount,
			).Write(selfLL))
			m.SetMetric("log_likelihood_gain_vs_self_only", data.NewMetric(
				"log_likelihood_gain_vs_self_only",
				data.UnitDimensionless,
				data.TimescaleInstantaneous,
				0.0,
				markedCount,
			).Write(gainSelf))
			m.SetMetric("log_likelihood_gain_per_event_vs_self_only", data.NewMetric(
				"log_likelihood_gain_per_event_vs_self_only",
				data.UnitDimensionless,
				data.TimescaleInstantaneous,
				0.0,
				1.0,
			).Write(gainSelf/markedCount))
		}
	}

	buySupport, sellSupport := streamPrior.kernelIntegralSupport(atSec, beta)
	compBuy := muX*spanPrior + (alphaXX/beta)*buySupport + (alphaXY/beta)*sellSupport
	compSell := muY*spanPrior + (alphaYX/beta)*buySupport + (alphaYY/beta)*sellSupport

	priorCountBuy := float64(len(buyArrivals))
	priorCountSell := float64(len(sellArrivals))
	innoBuy := priorCountBuy - compBuy
	innoSell := priorCountSell - compSell

	m.SetMetric("compensator:buy", data.NewMetric(
		"compensator:buy",
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
		compBuy,
		math.Sqrt(compBuy),
	).Write(compBuy))
	m.SetMetric("compensator:sell", data.NewMetric(
		"compensator:sell",
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
		compSell,
		math.Sqrt(compSell),
	).Write(compSell))
	m.SetMetric("count_innovation:buy", data.NewMetric(
		"count_innovation:buy",
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
		0.0,
		math.Sqrt(compBuy),
	).Write(innoBuy))
	m.SetMetric("count_innovation:sell", data.NewMetric(
		"count_innovation:sell",
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
		0.0,
		math.Sqrt(compSell),
	).Write(innoSell))

	if compBuy > 0 {
		m.SetMetric("standardized_innovation:buy", data.NewMetric(
			"standardized_innovation:buy",
			data.UnitDimensionless,
			data.TimescaleInstantaneous,
			0.0,
			1.0,
		).Write(innoBuy/math.Sqrt(compBuy)))
	}

	if compSell > 0 {
		m.SetMetric("standardized_innovation:sell", data.NewMetric(
			"standardized_innovation:sell",
			data.UnitDimensionless,
			data.TimescaleInstantaneous,
			0.0,
			1.0,
		).Write(innoSell/math.Sqrt(compSell)))
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

	m.SetMetric("excitation_mass:buy", data.NewMetric(
		"excitation_mass:buy",
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
		0.0,
		compBuy,
	).Write(excessBuyMass))
	m.SetMetric("excitation_mass:sell", data.NewMetric(
		"excitation_mass:sell",
		data.UnitDimensionless,
		data.TimescaleInstantaneous,
		0.0,
		compSell,
	).Write(excessSellMass))

	if compBuy > 0 {
		m.SetMetric("excitation_share:buy", data.NewMetric(
			"excitation_share:buy",
			data.UnitDimensionless,
			data.TimescaleInstantaneous,
			0.5,
			0.5,
		).Write(excessBuyMass/compBuy))
	}

	if compSell > 0 {
		m.SetMetric("excitation_share:sell", data.NewMetric(
			"excitation_share:sell",
			data.UnitDimensionless,
			data.TimescaleInstantaneous,
			0.5,
			0.5,
		).Write(excessSellMass/compSell))
	}

	if compTotal := compBuy + compSell; compTotal > 0 {
		m.SetMetric("excitation_share", data.NewMetric(
			"excitation_share",
			data.UnitDimensionless,
			data.TimescaleInstantaneous,
			0.5,
			0.5,
		).Write((excessBuyMass+excessSellMass)/compTotal))
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

		m.SetMetric("snr", data.NewMetric(
			"snr",
			data.UnitDimensionless,
			data.TimescaleInstantaneous,
			0.0,
			1.0,
		).Write(p.snr))
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
			m := *(**data.Measurement)(arriving)
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
