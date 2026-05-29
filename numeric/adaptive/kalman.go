package adaptive

import "math"

/*
ScalarKalman is a one-dimensional Kalman filter for a latent random-walk state
observed through noisy measurements. It is the scalar specialisation of the
predict/update recursion, so there are no matrices to invert and the per-update
cost is a handful of floating point operations — cheap enough for the hot path.

The novelty over a plain EMA is that the measurement noise is learned, not fixed.
The filter tracks the empirical power of its own innovations (the gap between a
measurement and the prior state estimate) and inflates the noise of any single
measurement that lands many standard deviations out. A flash-crash tick therefore
moves the state a little instead of destroying it, and once the innovation stream
calms down the gain climbs back to its steady-state value on its own. This is the
asymmetry a symmetric EMA cannot express: shocks are absorbed, recovery is fast.

The caller supplies the calm-market gain per update (the gain the filter would use
if the measurement were perfectly ordinary). Internally that gain is the analogue
of the steady-state Kalman gain g = P/(P+R); an anomalous measurement has its noise
scaled by 1/w for a robustness weight w in (0, 1], collapsing the effective gain to
g·w / (g·w + (1-g)). w = 1 reproduces the plain EMA exactly.
*/
type ScalarKalman struct {
	state       float64
	scaleVar    AlphaEMA // EWMA of winsorised innovation^2: the expected innovation power
	scaleRate   float64
	initialVar  float64
	warmup      int
	updates     int
	initialized bool
}

/*
NewScalarKalman builds a filter. initialSigma is the prior standard deviation of a
measurement around the state before any innovations have been seen; it seeds the
adaptive noise estimate so the very first innovations are scored sensibly. scaleRate
is the smoothing rate for the innovation-power tracker (higher tracks regime shifts
faster, lower is more robust). warmup measurements pass through at the full calm gain
before robust down-weighting engages, so the scale estimate has data to stand on.
*/
func NewScalarKalman(initialSigma, scaleRate float64, warmup int) *ScalarKalman {
	if initialSigma <= 0 {
		initialSigma = 1
	}

	if scaleRate <= 0 || scaleRate > 1 {
		scaleRate = 0.2
	}

	if warmup < 1 {
		warmup = 1
	}

	return &ScalarKalman{
		scaleRate:  scaleRate,
		initialVar: initialSigma * initialSigma,
		warmup:     warmup,
	}
}

/*
Observe folds one measurement using calmGain as the steady-state blend weight and
sigmaCutoff as the robust threshold in standard deviations (a Huber influence bound).
It returns the updated state and the standardised innovation magnitude (|z-score|),
which callers use to detect shocks and gate recovery. The first measurement seeds
the state. calmGain is clamped to (0, 1]; a non-positive sigmaCutoff disables robust
down-weighting (pure adaptive EMA).
*/
func (kalman *ScalarKalman) Observe(measurement, calmGain, sigmaCutoff float64) (float64, float64) {
	if calmGain <= 0 {
		calmGain = 1e-9
	}

	if calmGain > 1 {
		calmGain = 1
	}

	kalman.updates++

	if !kalman.initialized {
		kalman.state = measurement
		_ = kalman.scaleVar.Update(kalman.initialVar, 1)
		kalman.initialized = true

		return kalman.state, 0
	}

	innovation := measurement - kalman.state
	sigma := math.Sqrt(math.Max(kalman.scaleVar.Value(), 0))

	zscore := 0.0
	if sigma > 0 {
		zscore = math.Abs(innovation) / sigma
	}

	weight := 1.0
	if sigmaCutoff > 0 && kalman.updates > kalman.warmup && zscore > sigmaCutoff {
		// Huber influence: a measurement sigmaCutoff*k sigmas out contributes as if
		// it were only sigmaCutoff sigmas out, so its noise is inflated by zscore/cutoff.
		weight = sigmaCutoff / zscore
	}

	// Effective gain after inflating this measurement's noise by 1/weight. Equivalent
	// to g·w / (g·w + (1-g)) with g the calm-market Kalman gain.
	scaledGain := calmGain * weight
	gain := scaledGain / (scaledGain + (1 - calmGain))

	kalman.state += gain * innovation

	// Update the innovation-power estimate with a winsorised residual so transient
	// spikes do not permanently distort the noise scale, yet a persistent regime
	// shift still grows it (the state itself migrates, so residuals shrink in turn).
	winsorised := innovation
	if weight < 1 {
		winsorised = innovation * weight
	}
	_ = kalman.scaleVar.Update(winsorised*winsorised, kalman.scaleRate)

	return kalman.state, zscore
}

/*
State returns the current latent estimate. Before the first measurement it is zero.
*/
func (kalman *ScalarKalman) State() float64 {
	return kalman.state
}

/*
Updates counts measurements folded in, including the seeding measurement.
*/
func (kalman *ScalarKalman) Updates() int {
	return kalman.updates
}

/*
Sigma returns the current estimated innovation standard deviation.
*/
func (kalman *ScalarKalman) Sigma() float64 {
	return math.Sqrt(math.Max(kalman.scaleVar.Value(), 0))
}

/*
Reset clears the filter to its initial, unseeded state.
*/
func (kalman *ScalarKalman) Reset() {
	kalman.state = 0
	kalman.updates = 0
	kalman.initialized = false
	kalman.scaleVar.Reset()
}
