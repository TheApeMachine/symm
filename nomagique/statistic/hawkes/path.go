package hawkes

import "time"

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
	// What the fitted process, a plain Poisson process, and a process whose
	// directions do not excite each other each said the observed arrivals
	// were worth. Their differences are how much the self-excitation and the
	// cross-excitation actually bought.
	hawkesLogLikelihood   float64
	poissonLogLikelihood  float64
	selfOnlyLogLikelihood float64
	likelihoodsReady      bool
	snr                   float64
	hasSNR                bool
}

/*
newPath creates an isolated point process path state.
*/
func newPath() *path {
	return &path{samples: make([]sample, 0)}
}

/*
sides splits the retained history into per-side seconds positions.
*/
func (path *path) sides() (buy []float64, sell []float64) {
	buy = make([]float64, 0, len(path.samples))
	sell = make([]float64, 0, len(path.samples))

	for _, s := range path.samples {
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
func (path *path) origin() time.Time {
	return path.samples[0].at
}

/*
remember incorporates one accepted arrival into the history, evicting the
oldest once the retained path is at capacity.
*/
func (path *path) remember(at time.Time, atSec float64, mark float64) {
	path.samples = append(path.samples, sample{at: at, atSec: atSec, mark: mark})
}

/*
refit re-estimates the bivariate model (and its self-only restriction) from
the retained history, keeping the previous model when the data cannot
identify a new one. Every bound, grid, and cadence gate comes from the
observed events through the fit context.
*/
func (path *path) refit(atSec float64) {
	if len(path.samples) >= 2 {
		if path.samples[len(path.samples)-1].at.Equal(path.samples[len(path.samples)-2].at) {
			return
		}
	}

	buyArrivals, sellArrivals := path.sides()

	stream := newArrivalStream(buyArrivals, sellArrivals)
	context, ok := newFitContext(stream, atSec)

	if !ok || !context.enoughEvents(stream) {
		return
	}

	path.eventsSinceFit++

	if path.modelReady && path.eventsSinceFit < context.minFitEvents {
		return
	}

	prior := bivariateFit{}

	if path.modelReady {
		prior = path.model
	}

	estimator := newBivariateEstimator(prior)
	fitted := estimator.fit(stream, atSec)

	if !fitted.valid() {
		return
	}

	path.model = fitted
	path.modelReady = true
	path.modelSupport = float64(context.totalEvents)
	path.eventsSinceFit = 0

	selfOnly := estimator.fitSelfOnly(stream, atSec)

	if selfOnly.valid() {
		path.selfOnlyModel = selfOnly
		path.selfOnlyReady = true
	}

	path.recordLikelihoods(stream, atSec, context, fitted, selfOnly)
}

/*
recordLikelihoods keeps what each competing description of the arrivals was
worth, so a caller can see whether the excitation earned its parameters.
*/
func (path *path) recordLikelihoods(
	stream arrivalStream,
	atSec float64,
	context fitContext,
	fitted bivariateFit,
	selfOnly bivariateFit,
) {
	path.likelihoodsReady = false

	hawkes, hawkesOK := fitted.logLikelihood(stream, atSec)

	if !hawkesOK {
		return
	}

	poisson, poissonOK := context.poissonFit().withIntensitiesAt(stream, atSec).
		logLikelihood(stream, atSec)

	if !poissonOK {
		return
	}

	path.hawkesLogLikelihood = hawkes
	path.poissonLogLikelihood = poisson
	path.selfOnlyLogLikelihood = poisson
	path.likelihoodsReady = true

	if !selfOnly.valid() {
		return
	}

	if value, ok := selfOnly.logLikelihood(stream, atSec); ok {
		path.selfOnlyLogLikelihood = value
	}
}
