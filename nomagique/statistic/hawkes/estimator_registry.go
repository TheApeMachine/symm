package hawkes

import "sync"

/*
Parameters is the fitted bivariate Hawkes excitation structure a consumer
reads back into its own Frame-shaped intensity primitive.
*/
type Parameters struct {
	AlphaAA float64
	AlphaAB float64
	AlphaBA float64
	AlphaBB float64
	Beta    float64
}

/*
symbolEstimator refits one symbol's bivariate Hawkes parameters from its own
retained arrival window, on its own data-derived retention horizon and
minimum-event gate. It holds no market-specific knowledge beyond the two
timestamp streams it is given.
*/
type symbolEstimator struct {
	mutex           sync.Mutex
	window          *arrivalWindow
	fitted          Parameters
	haveFitted      bool
	prior           bivariateFit
	eventsAtLastFit int
}

func newSymbolEstimator() *symbolEstimator {
	return &symbolEstimator{window: newArrivalWindow(0)}
}

/*
observe records one arrival and refits when the retained window both carries
enough events for the fit's own identifiability requirement and has grown by
at least that same requirement's own scale since the last refit. Multi-start
L-BFGS is too expensive to rerun on every single arrival; minFitEvents is
already the data-derived scale the fit itself uses to judge sufficiency, so
reusing it as the refit cadence introduces no additional constant.
*/
func (estimator *symbolEstimator) observe(atSec float64, buy bool) (Parameters, bool) {
	estimator.mutex.Lock()
	defer estimator.mutex.Unlock()

	if buy {
		estimator.window.appendBuy(atSec)
	} else {
		estimator.window.appendSell(atSec)
	}

	stream := estimator.window.stream()
	observation, ok := newObservationContext(stream, atSec)

	if !ok {
		return estimator.fitted, estimator.haveFitted
	}

	estimator.window.retainFrom(atSec - observation.tradeWindow.Seconds())

	if !observation.enoughEvents(stream) {
		return estimator.fitted, estimator.haveFitted
	}

	if estimator.haveFitted && observation.totalEvents-estimator.eventsAtLastFit < observation.minFitEvents {
		return estimator.fitted, true
	}

	fit := newBivariateEstimator(estimator.prior).fit(stream, atSec)

	if !fit.valid() {
		return estimator.fitted, estimator.haveFitted
	}

	estimator.prior = fit
	estimator.eventsAtLastFit = observation.totalEvents
	estimator.fitted = Parameters{
		AlphaAA: fit.alphaXX,
		AlphaAB: fit.alphaXY,
		AlphaBA: fit.alphaYX,
		AlphaBB: fit.alphaYY,
		Beta:    fit.beta,
	}
	estimator.haveFitted = true

	return estimator.fitted, true
}

/*
EstimatorRegistry refits bivariate Hawkes excitation parameters per symbol
from accumulated buy/sell arrival timestamps, so a market's own excitation
structure — not a fixed guess — drives its Hawkes intensity primitive.
*/
type EstimatorRegistry struct {
	estimators sync.Map
}

/*
NewEstimatorRegistry returns an empty per-symbol Hawkes estimator registry.
*/
func NewEstimatorRegistry() *EstimatorRegistry {
	return &EstimatorRegistry{}
}

/*
Observe records one arrival for symbol at atSec (seconds since an arbitrary
but consistent epoch) and returns the symbol's latest fitted parameters. ok
is false until the symbol's retained history first satisfies the fit's own
identifiability requirement; callers should keep using their fallback
parameters until then.
*/
func (registry *EstimatorRegistry) Observe(symbol string, atSec float64, buy bool) (Parameters, bool) {
	value, _ := registry.estimators.LoadOrStore(symbol, newSymbolEstimator())

	return value.(*symbolEstimator).observe(atSec, buy)
}
