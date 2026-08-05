package resonance

import "math"

/*
The learning pace is bounded so the controller cannot drive the network into
either degenerate regime. At the floor the network still adapts, slowly enough
to hold a model across a quiet stretch; at the ceiling it re-fits fast enough to
follow a regime change without the state update overshooting within a tick. The
rest pace sits nearer the floor than the ceiling because a market is stationary
more often than not, so resting slow and escalating on evidence costs less than
the reverse.
*/
const (
	restAlpha = 0.03
	minAlpha  = 0.005
	maxAlpha  = 0.150
)

/*
AlphaController sets the learning pace from how surprising the current
reconstruction error is relative to its own recent history.

The control signal is the rank of the reconstruction error within its own recent
history, not a ratio of raw error norms. Two properties follow that the
raw-ratio formulation could not provide.

It is scale-free. Error norms grow with the feature count and with market
volatility, so any fixed threshold on a raw norm means something different on
every schema and in every regime. A rank is expressed as a share of the retained
window, so the same thresholds hold as the schema settles and as volatility
moves.

It survives a sustained shift. A recursively adapted mean and variance, such as
adaptive.ZScore, tracks a step change into its own centre within a tick or two:
once the reading stops moving, the adaptation rate falls to zero, the mean
settles on the new level and the score collapses. That is the correct behaviour
for detecting a change in level, and exactly wrong here, because a regime the
network cannot predict is a sustained condition and the pace must stay elevated
for as long as it lasts. Ranking against a retained window keeps a shifted
reading at the top of its distribution until the window itself turns over.

It has a fixed point. The controller pulls alpha toward a resting pace whenever
the error sits within its normal band, so a quiet market returns the pace to
rest instead of leaving it wherever the last excursion left it. The previous
formulation was multiplicative in one direction per branch with no restoring
term, which made it a ratchet: any market with intermittent spikes walked alpha
to its ceiling and held it there.

Surprise raises the pace, because a run of unusually large errors means the
retained model no longer describes the market and should be replaced faster.
Unusually small errors lower it, because the model is tracking and the remaining
error is mostly noise that a high pace would fit.
*/
type AlphaController struct {
	restAlpha    float64
	currentAlpha float64
	minAlpha     float64
	maxAlpha     float64
	logAlpha     float64
	logRest      float64
	logMin       float64
	logMax       float64
	surprise     *errorCalibrator
}

/*
alphaControllerGain is how far one fully surprising observation may move the
pace, as a fraction of the distance to the bound it moves toward.

The pace is a property of the market's stationarity, which changes over many
ticks, while the control signal is computed per tick. A gain near one would let a
single outlier tick reset the pace outright; a gain near zero would make the
controller unable to respond to a genuine regime change within the horizon the
forecast is used over. A tenth means a sustained excursion converges over roughly
the same number of ticks as the forward horizon the solver rolls out, and a lone
outlier moves the pace by a tenth of the way and is then pulled back.
*/
const alphaControllerGain = 0.1

/*
alphaControllerBand is the tail share of the retained error distribution that
counts as evidence about the pace.

An error in the worst fifth of recent history says the retained model is
struggling and the pace should rise; one in the best fifth says it is tracking
and the pace may fall. Everything between is ordinary, and treating ordinary
observations as evidence is what produces drift. A fifth on each side leaves
three fifths of observations neutral, so the pace responds to genuine tails
rather than to routine variation, and the two tails are the same size so a
symmetric error stream produces no net movement.
*/
const alphaControllerBand = 0.2

/*
NewAlphaController constructs a dynamic pace controller bounded within
[minAlpha, maxAlpha] and resting at initialAlpha.
*/
func NewAlphaController(initialAlpha, minAlpha, maxAlpha float64) *AlphaController {
	return &AlphaController{
		restAlpha:    initialAlpha,
		currentAlpha: initialAlpha,
		minAlpha:     minAlpha,
		maxAlpha:     maxAlpha,
		logAlpha:     math.Log(initialAlpha),
		logRest:      math.Log(initialAlpha),
		logMin:       math.Log(minAlpha),
		logMax:       math.Log(maxAlpha),
		surprise:     newErrorCalibrator(),
	}
}

/*
Update folds one reconstruction error into the retained dispersion estimate and
returns the pace it implies.

The temporal error is accepted so callers may pass it, but the pace is driven by
the reconstruction error alone. The two are not independent measurements of
different things: the temporal residual is a component of the same variational
energy, and the ratio between them is dominated by the relative dimensions of
the input and top layers rather than by anything about the market.

The pace is integrated in log space. Alpha is a multiplicative rate whose bounds
are not equidistant from rest, so a step of fixed size taken toward maxAlpha
moves the pace much further than the same step taken toward minAlpha. Under any
error stream that crosses the band symmetrically, that asymmetry integrates into
a net upward drift, which is a slower version of the ratchet this controller
replaced. In log space the bounds sit at comparable distances, an up step and a
down step of equal magnitude compose to no change, and a symmetric stream
therefore holds the pace at rest.
*/
func (controller *AlphaController) Update(reconstructionError, temporalError float64) float64 {
	// Until the window holds enough history to rank against, there is no
	// evidence to act on and the pace stays where it is. Warmup follows every
	// schema change, so this is a normal path rather than a failure.
	if controller.surprise.count() < errorCalibratorWindow {
		controller.surprise.Quantile(reconstructionError)

		return controller.currentAlpha
	}

	// The quantile is the share of recent errors this one beats, so a small
	// value means the error is among the worst seen lately and a large value
	// means it is among the best.
	rank := controller.surprise.Quantile(reconstructionError)
	target := controller.logRest

	switch {
	case rank < alphaControllerBand:
		target = controller.logMax
	case rank > 1.0-alphaControllerBand:
		target = controller.logMin
	}

	controller.logAlpha += alphaControllerGain * (target - controller.logAlpha)
	controller.logAlpha = min(controller.logMax, max(controller.logMin, controller.logAlpha))
	controller.currentAlpha = math.Exp(controller.logAlpha)

	return min(controller.maxAlpha, max(controller.minAlpha, controller.currentAlpha))
}
