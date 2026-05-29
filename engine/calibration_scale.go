package engine

import "github.com/theapemachine/symm/numeric/adaptive"

const (
	// calibrationInitialSigma is the prior innovation standard deviation of the
	// actual/predicted ratio, which lives in [0, maxSampleRatio] centred near one.
	calibrationInitialSigma = 0.3
	// calibrationScaleRate smooths the innovation-power tracker that drives the
	// adaptive measurement noise; moderate so transient spikes are absorbed but a
	// genuine regime shift is eventually tracked.
	calibrationScaleRate = 0.2
	// calibrationWarmup measurements pass through at the full calm gain before robust
	// shock down-weighting engages, so the noise scale has data to stand on.
	calibrationWarmup = 5
)

/*
calibrationGateParams configures the asymmetric, volatility-gated gain that wraps the
scalar Kalman filter. shockSigma is the robust threshold (standard deviations) beyond
which a measurement is treated as a shock and absorbed rather than chased. recoveryFactor
multiplies the upside gain once a shock has passed and the scale is still depressed, so
the signal climbs back to trust quickly instead of starving. recoveryBand is how far below
its healthy baseline the scale must sit to count as depressed. recoverySamples is how many
consecutive calm measurements must arrive before the recovery boost opens. baselineAlpha is
the smoothing of the slow baseline that remembers the pre-shock healthy scale.
*/
type calibrationGateParams struct {
	shockSigma      float64
	recoveryFactor  float64
	recoveryBand    float64
	recoverySamples int
	baselineAlpha   float64
}

/*
calibrationScale tracks the per-regime actual/predicted ratio with a robust scalar Kalman
filter under an asymmetric, volatility-gated gain.

The symmetry the old EWMA could not break was that the descent into pessimism after a shock
was governed by the same half-life as the climb back out, and because no trades are taken at
low trust, no fresh feedback arrives to lift it — a starvation loop. Here the asymmetry is
explicit. Downside surprises move the scale at the full calm gain immediately (capital
protection), while the Kalman's adaptive measurement noise keeps a single flash-crash tick
from collapsing the state. Upside moves run at the calm gain in normal conditions, but once a
shock has passed and the scale is still sitting below its healthy baseline, the upside gain is
multiplied by recoveryFactor so a handful of good settled predictions rehabilitate the signal
fast.
*/
type calibrationScale struct {
	kalman   *adaptive.ScalarKalman
	baseline adaptive.AlphaEMA
	cfg      calibrationGateParams
	calmRun  int
}

func newCalibrationScale(cfg calibrationGateParams) *calibrationScale {
	if cfg.shockSigma <= 0 {
		cfg.shockSigma = 3
	}

	if cfg.recoveryFactor < 1 {
		cfg.recoveryFactor = 1
	}

	if cfg.recoveryBand < 0 {
		cfg.recoveryBand = 0
	}

	if cfg.recoverySamples < 1 {
		cfg.recoverySamples = 1
	}

	if cfg.baselineAlpha <= 0 || cfg.baselineAlpha > 1 {
		cfg.baselineAlpha = 0.05
	}

	return &calibrationScale{
		kalman: adaptive.NewScalarKalman(calibrationInitialSigma, calibrationScaleRate, calibrationWarmup),
		cfg:    cfg,
	}
}

/*
Observe folds one calibration sample (an actual/predicted ratio) using calmGain as the
steady-state blend weight derived from the regime half-life. It returns the live scale.
*/
func (scale *calibrationScale) Observe(sample, calmGain float64) float64 {
	gain := scale.gateGain(sample, calmGain)

	state, zscore := scale.kalman.Observe(sample, gain, scale.cfg.shockSigma)

	if zscore <= scale.cfg.shockSigma {
		scale.calmRun++
	} else {
		scale.calmRun = 0
	}

	_ = scale.baseline.Update(state, scale.cfg.baselineAlpha)

	return scale.Scale()
}

/*
gateGain applies the directional and recovery asymmetry to the calm gain before it reaches
the filter. Downside is always responsive; upside is responsive only once recovery is open.
*/
func (scale *calibrationScale) gateGain(sample, calmGain float64) float64 {
	if scale.kalman.Updates() == 0 {
		return calmGain
	}

	if sample < scale.kalman.State() {
		return calmGain
	}

	if !scale.recoveryOpen() {
		return calmGain
	}

	boosted := calmGain * scale.cfg.recoveryFactor

	if boosted > 1 {
		return 1
	}

	return boosted
}

/*
recoveryOpen reports whether the scale is sitting below its healthy baseline after a run of
calm measurements — the window in which upside feedback should be fast-tracked.
*/
func (scale *calibrationScale) recoveryOpen() bool {
	if scale.kalman.Updates() <= calibrationWarmup {
		return false
	}

	if scale.calmRun < scale.cfg.recoverySamples {
		return false
	}

	baseline := scale.baseline.Value()

	if baseline <= 0 {
		return false
	}

	return scale.kalman.State() < baseline*(1-scale.cfg.recoveryBand)
}

/*
Scale returns the live parameter multiplier. Before the first sample it is one; a
non-positive estimate floors to zero, matching the prior calibration semantics.
*/
func (scale *calibrationScale) Scale() float64 {
	if scale.kalman.Updates() == 0 {
		return 1
	}

	value := scale.kalman.State()

	if value <= 0 {
		return 0
	}

	return value
}

/*
Updates counts settled samples folded into the scale, including the seeding sample.
*/
func (scale *calibrationScale) Updates() int {
	return scale.kalman.Updates()
}
