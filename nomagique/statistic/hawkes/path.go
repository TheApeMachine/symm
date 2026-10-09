package hawkes

import (
	"time"

	"github.com/theapemachine/errnie"
)

/*
MaxArrivalSamples is the retained arrival history's capacity per symbol: past
it, the oldest arrival is evicted on every new one and the fit sees a rolling
window.
*/
const MaxArrivalSamples = 64

/*
sample is one retained arrival: its venue timestamp, its seconds position on
the path's relative epoch, and its side mark.
*/
type sample struct {
	at    time.Time
	atSec float64
	mark  float64
}

/*
path is one symbol's stage-internal estimation state: the retained arrival
history and the fitted models those arrivals have produced. A model published
for an event is always the one fitted strictly before that event arrived; a
refit triggered by incorporating an event only takes effect for the next one.
*/
type path struct {
	samples        []sample
	lastAt         time.Time
	hasLast        bool
	model          bivariateFit
	modelReady     bool
	selfOnlyModel  bivariateFit
	selfOnlyReady  bool
	eventsSinceFit int
	modelSupport   float64
	fitAtSec       float64
	fitSpanSec     float64
}

/*
sides splits the retained history into per-side seconds positions.
*/
func (p *path) sides() (buy []float64, sell []float64) {
	buy = make([]float64, 0, len(p.samples))
	sell = make([]float64, 0, len(p.samples))

	for _, s := range p.samples {
		if s.mark > 0 {
			buy = append(buy, s.atSec)
			continue
		}

		sell = append(sell, s.atSec)
	}

	return buy, sell
}

/*
origin returns the earliest retained arrival, the observation window's start.
*/
func (p *path) origin() time.Time {
	return p.samples[0].at
}

/*
remember incorporates one accepted arrival into the history, evicting the
oldest once the retained path is at capacity.
*/
func (p *path) remember(at time.Time, atSec float64, mark float64) {
	if len(p.samples) >= MaxArrivalSamples {
		p.samples = p.samples[1:]
	}

	p.samples = append(p.samples, sample{at: at, atSec: atSec, mark: mark})
}

/*
refit re-estimates the bivariate model (and its self-only restriction) from
the retained history. Between refits the published model is the last one
fitted, on the cadence the fit context derives from the observed events, but
never once more time has passed since the fit than the span it was fitted on:
past that the retained window describes a period the model never saw, and its
rates would be stretched over it. When the retained history can no longer
identify a model, or the fit itself fails, the stale model is dropped rather
than kept: model outputs are then undefined until a fit succeeds again, and
the returned error says why the model went.
*/
func (p *path) refit(atSec float64) error {
	if len(p.samples) >= 2 {
		if p.samples[len(p.samples)-1].at.Equal(p.samples[len(p.samples)-2].at) {
			return nil
		}
	}

	buyArrivals, sellArrivals := p.sides()

	stream := newArrivalStream(buyArrivals, sellArrivals)
	context, ok := newFitContext(stream, atSec)

	if !ok || !context.enoughEvents(stream) {
		return p.drop("hawkes: retained arrivals no longer identify a model")
	}

	p.eventsSinceFit++

	if p.modelReady &&
		p.eventsSinceFit < context.minFitEvents &&
		atSec-p.fitAtSec < p.fitSpanSec {
		return nil
	}

	prior := bivariateFit{}

	if p.modelReady {
		prior = p.model
	}

	estimator := newBivariateEstimator(prior)
	fitted := estimator.fit(stream, atSec)

	if !fitted.valid() {
		return p.drop("hawkes: refit produced no valid model")
	}

	p.model = fitted
	p.modelReady = true
	p.modelSupport = float64(context.totalEvents)
	p.eventsSinceFit = 0
	p.fitAtSec = atSec
	p.fitSpanSec = context.spanSec

	selfOnly := estimator.fitSelfOnly(stream, atSec)
	p.selfOnlyModel = selfOnly
	p.selfOnlyReady = selfOnly.valid()

	return nil
}

/*
drop discards the published model. It reports the reason only when a model
was actually discarded, so warm-up and a still-unidentifiable history do not
repeat the same report on every arrival. The caller decides how to log it.
*/
func (p *path) drop(reason string) error {
	if !p.modelReady {
		return nil
	}

	p.model = bivariateFit{}
	p.modelReady = false
	p.selfOnlyModel = bivariateFit{}
	p.selfOnlyReady = false
	p.eventsSinceFit = 0
	p.fitAtSec = 0
	p.fitSpanSec = 0

	return errnie.Err(errnie.Validation, reason, nil)
}
