package hawkes

import "time"

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
	snr            float64
	hasSNR         bool
}

/*
paths owns every symbol's path. The pipeline's stages share one registry so a
measurement's Label addresses the same arrival history in every stage.
*/
type paths struct {
	byLabel map[string]*path
}

/*
newPaths creates the shared per-symbol registry.
*/
func newPaths() *paths {
	return &paths{byLabel: make(map[string]*path)}
}

/*
at resolves one symbol's path, creating it on first sight.
*/
func (registry *paths) at(label string) *path {
	existing, found := registry.byLabel[label]

	if found {
		return existing
	}

	fresh := &path{samples: make([]sample, 0, MaxArrivalSamples)}
	registry.byLabel[label] = fresh

	return fresh
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
the retained history, keeping the previous model when the data cannot
identify a new one. Every bound, grid, and cadence gate comes from the
observed events through the fit context.
*/
func (p *path) refit(atSec float64) {
	if len(p.samples) >= 2 {
		if p.samples[len(p.samples)-1].at.Equal(p.samples[len(p.samples)-2].at) {
			return
		}
	}

	buyArrivals, sellArrivals := p.sides()

	stream := newArrivalStream(buyArrivals, sellArrivals)
	context, ok := newFitContext(stream, atSec)

	if !ok || !context.enoughEvents(stream) {
		return
	}

	p.eventsSinceFit++

	if p.modelReady && p.eventsSinceFit < context.minFitEvents {
		return
	}

	prior := bivariateFit{}

	if p.modelReady {
		prior = p.model
	}

	estimator := newBivariateEstimator(prior)
	fitted := estimator.fit(stream, atSec)

	if !fitted.valid() {
		return
	}

	p.model = fitted
	p.modelReady = true
	p.modelSupport = float64(context.totalEvents)
	p.eventsSinceFit = 0

	selfOnly := estimator.fitSelfOnly(stream, atSec)

	if selfOnly.valid() {
		p.selfOnlyModel = selfOnly
		p.selfOnlyReady = true
	}
}
