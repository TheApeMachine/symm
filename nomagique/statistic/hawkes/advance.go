package hawkes

import (
	"fmt"
	"iter"
	"math"
	"time"
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
regressing event time, then publishes the empirical counts, fractions, and
arrival rates the window supports, naming the window's start as "from".
*/
type Counts struct {
	*core.PrimitiveError
	history *paths
	label   string
	input   data.Map[string]
	output  data.Map[float64]
}

/*
NewCounts creates the empirical arrival stage over the shared registry.
*/
func NewCounts(history *paths, label string) core.Primitive {
	return &Counts{
		PrimitiveError: core.NewPrimitiveError(),
		history:        history,
		label:          label,
		input:          data.NewMap("buy", "buy", "sell", "sell", "at", "at"),
		output:         data.NewOutputMap(),
	}
}

func (op *Counts) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			buy, buyOK := values.Values["buy"]
			sell, sellOK := values.Values["sell"]
			atSec, atOK := values.Values["at"]

			if !buyOK || !sellOK || !atOK {
				op.Error(core.ErrNotHeld)
				return
			}

			at := time.Unix(0, int64(atSec*1e9)).UTC()
			mark := -1.0

			if buy == 1 {
				mark = 1.0
			}

			_ = sell

			p := op.history.at(op.label)

			if p.hasLast && at.Before(p.lastAt) {
				op.Error(fmt.Errorf("%w: hawkes: regressing event time", core.ErrDomain))
				return
			}

			p.lastAt = at
			p.hasLast = true

			buyArrivals, sellArrivals := p.sides()

			countBuy := float64(len(buyArrivals))
			countSell := float64(len(sellArrivals))

			if mark > 0 {
				countBuy++
			} else {
				countSell++
			}

			count := countBuy + countSell
			from := at
			fromSec := atSec

			if len(p.samples) > 0 {
				from = p.origin()
				fromSec = float64(from.UnixNano()) * 1e-9
			}

			span := atSec - fromSec

			clear(op.output.Values)
			op.output.Values["from"] = fromSec
			op.output.Values["event_count"] = count
			op.output.Values["event_count:buy"] = countBuy
			op.output.Values["event_count:sell"] = countSell
			op.output.Values["event_fraction:buy"] = countBuy / count
			op.output.Values["event_fraction:sell"] = countSell / count

			if span > 0 {
				op.output.Values["arrival_rate:buy"] = countBuy / span
				op.output.Values["arrival_rate:sell"] = countSell / span
				op.output.Values["arrival_rate"] = (countBuy + countSell) / span
			}

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
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
is nothing to measure against and the arrival moves through untouched.
*/
type Excitation struct {
	*core.PrimitiveError
	history *paths
	label   string
	input   data.Map[string]
	output  data.Map[float64]
}

/*
NewExcitation creates the model-evaluation stage over the shared registry.
*/
func NewExcitation(history *paths, label string) core.Primitive {
	return &Excitation{
		PrimitiveError: core.NewPrimitiveError(),
		history:        history,
		label:          label,
		input:          data.NewMap("buy", "buy", "sell", "sell", "at", "at"),
		output:         data.NewOutputMap(),
	}
}

func (op *Excitation) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			p := op.history.at(op.label)

			if !p.modelReady {
				if !yield(arriving) {
					return
				}

				continue
			}

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			buy, buyOK := values.Values["buy"]
			_, sellOK := values.Values["sell"]
			atSec, atOK := values.Values["at"]

			if !buyOK || !sellOK || !atOK {
				op.Error(core.ErrNotHeld)
				return
			}

			mark := -1.0

			if buy == 1 {
				mark = 1.0
			}

			buyArrivals, sellArrivals := p.sides()

			clear(op.output.Values)
			op.evaluate(p, buyArrivals, sellArrivals, atSec, mark)

			if len(op.output.Values) > 0 {
				for range adapter.Next(data.NewValue(op.output)) {
				}

				if err := adapter.Error(); err != nil {
					op.Error(err)
					return
				}
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

/*
evaluate publishes one event's model-conditioned facts into op.output.
*/
func (op *Excitation) evaluate(p *path, buyArrivals, sellArrivals []float64, atSec, mark float64) {
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

	op.output.Values["conditional_intensity:buy"] = lambdaBuy
	op.output.Values["conditional_intensity:sell"] = lambdaSell
	op.output.Values["conditional_intensity"] = lambdaBuy + lambdaSell
	op.output.Values["background_rate:buy"] = muX
	op.output.Values["background_rate:sell"] = muY
	op.output.Values["background_rate"] = muX + muY
	op.output.Values["excitation_intensity:buy"] = excessBuy
	op.output.Values["excitation_intensity:sell"] = excessSell

	if lambdaBuy > 0 {
		op.output.Values["excitation_fraction:buy"] = excessBuy / lambdaBuy
	}

	if lambdaSell > 0 {
		op.output.Values["excitation_fraction:sell"] = excessSell / lambdaSell
	}

	op.output.Values["excitation_amplitude:buy_from_buy"] = alphaXX
	op.output.Values["excitation_amplitude:buy_from_sell"] = alphaXY
	op.output.Values["excitation_amplitude:sell_from_buy"] = alphaYX
	op.output.Values["excitation_amplitude:sell_from_sell"] = alphaYY

	if beta > 0 {
		timescale := 1.0 / beta

		op.output.Values["excitation_decay"] = beta
		op.output.Values["excitation_decay:buy_from_buy"] = beta
		op.output.Values["excitation_decay:buy_from_sell"] = beta
		op.output.Values["excitation_decay:sell_from_buy"] = beta
		op.output.Values["excitation_decay:sell_from_sell"] = beta
		op.output.Values["excitation_timescale"] = timescale
		op.output.Values["excitation_timescale:buy_from_buy"] = timescale
		op.output.Values["excitation_timescale:buy_from_sell"] = timescale
		op.output.Values["excitation_timescale:sell_from_buy"] = timescale
		op.output.Values["excitation_timescale:sell_from_sell"] = timescale
	}

	matrix := branchingMatrix(alphaXX, alphaXY, alphaYX, alphaYY, beta)

	op.output.Values["offspring:buy_from_buy"] = matrix[0][0]
	op.output.Values["offspring:buy_from_sell"] = matrix[0][1]
	op.output.Values["offspring:sell_from_buy"] = matrix[1][0]
	op.output.Values["offspring:sell_from_sell"] = matrix[1][1]
	op.output.Values["branching_spectral_radius"] = spectralRadius(matrix)

	buyParent, sellParent, hasDesc := totalDescendants(alphaXX, alphaXY, alphaYX, alphaYY, beta)

	if hasDesc {
		op.output.Values["expected_descendants_from_buy"] = buyParent
		op.output.Values["expected_descendants_from_sell"] = sellParent
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
		op.output.Values["log_likelihood:hawkes"] = hawkesLL
		op.output.Values["log_likelihood_per_event:hawkes"] = hawkesLL / markedCount
	}

	poisson := bivariateFit{muX: muX, muY: muY, beta: beta}
	poissonLL, poissonOK := poisson.logLikelihood(streamWindow, atSec)

	if poissonOK {
		op.output.Values["log_likelihood:poisson"] = poissonLL
	}

	if hawkesOK && poissonOK {
		gainPoisson := hawkesLL - poissonLL
		op.output.Values["log_likelihood_gain_vs_poisson"] = gainPoisson
		op.output.Values["log_likelihood_gain_per_event_vs_poisson"] = gainPoisson / markedCount
	}

	if hawkesOK && p.selfOnlyReady {
		selfLL, selfOK := p.selfOnlyModel.logLikelihood(streamWindow, atSec)

		if selfOK {
			gainSelf := hawkesLL - selfLL
			op.output.Values["log_likelihood:self_only"] = selfLL
			op.output.Values["log_likelihood_gain_vs_self_only"] = gainSelf
			op.output.Values["log_likelihood_gain_per_event_vs_self_only"] = gainSelf / markedCount
		}
	}

	buySupport, sellSupport := streamPrior.kernelIntegralSupport(atSec, beta)
	compBuy := muX*spanPrior + (alphaXX/beta)*buySupport + (alphaXY/beta)*sellSupport
	compSell := muY*spanPrior + (alphaYX/beta)*buySupport + (alphaYY/beta)*sellSupport

	priorCountBuy := float64(len(buyArrivals))
	priorCountSell := float64(len(sellArrivals))
	innoBuy := priorCountBuy - compBuy
	innoSell := priorCountSell - compSell

	op.output.Values["compensator:buy"] = compBuy
	op.output.Values["compensator:sell"] = compSell
	op.output.Values["count_innovation:buy"] = innoBuy
	op.output.Values["count_innovation:sell"] = innoSell

	if compBuy > 0 {
		op.output.Values["standardized_innovation:buy"] = innoBuy / math.Sqrt(compBuy)
	}

	if compSell > 0 {
		op.output.Values["standardized_innovation:sell"] = innoSell / math.Sqrt(compSell)
	}

	// excitation_share is the excitation's share of the integrated
	// intensity over the whole observation span, where excitation_fraction
	// above is that share at this one instant.
	excessBuyMass := compBuy - muX*spanPrior
	excessSellMass := compSell - muY*spanPrior

	op.output.Values["excitation_mass:buy"] = excessBuyMass
	op.output.Values["excitation_mass:sell"] = excessSellMass

	if compBuy > 0 {
		op.output.Values["excitation_share:buy"] = excessBuyMass / compBuy
	}

	if compSell > 0 {
		op.output.Values["excitation_share:sell"] = excessSellMass / compSell
	}

	if compTotal := compBuy + compSell; compTotal > 0 {
		op.output.Values["excitation_share"] = (excessBuyMass + excessSellMass) / compTotal
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
		op.output.Values["snr"] = p.snr
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
	label   string
	input   data.Map[string]
}

/*
NewRefit creates the history-advance stage over the shared registry.
*/
func NewRefit(history *paths, label string) core.Primitive {
	return &Refit{
		PrimitiveError: core.NewPrimitiveError(),
		history:        history,
		label:          label,
		input:          data.NewMap("buy", "buy", "sell", "sell", "at", "at"),
	}
}

func (op *Refit) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			buy, buyOK := values.Values["buy"]
			_, sellOK := values.Values["sell"]
			atSec, atOK := values.Values["at"]

			if !buyOK || !sellOK || !atOK {
				op.Error(core.ErrNotHeld)
				return
			}

			at := time.Unix(0, int64(atSec*1e9)).UTC()
			mark := -1.0

			if buy == 1 {
				mark = 1.0
			}

			p := op.history.at(op.label)

			p.remember(at, atSec, mark)
			p.refit(atSec)

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
