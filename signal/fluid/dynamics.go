package fluid

import (
	"math"
	"time"

	"github.com/theapemachine/nomagique/statistic"
)

/*
fluidDynamics holds the per-symbol rolling baselines the classifier scales
against. Every series is recorded together once per Reading, so a single stamp
trail drives retention: the window depth is derived from observed event cadence
through nomagique statistic windows rather than a fixed ring capacity, and the first
observation already participates in scoring (no warmup sample gate).
*/
type fluidDynamics struct {
	stamps             []float64
	reynoldsHistory    []float64
	divergenceHistory  []float64
	viscosityHistory   []float64
	vorticityHistory   []float64
	turbulenceHistory  []float64
	sourceBalanceRatio []float64
}

/*
record appends one coherent dynamics row stamped at the event time, then trims
every series to the cadence-derived window depth. Series whose value failed its
finiteness guard append NaN-free by carrying their prior length forward via the
caller's guards; callers pass already-validated values.
*/
func (dynamics *fluidDynamics) record(
	at time.Time,
	reynolds, divergence, viscosity, vorticity, turbulence float64,
	addRate, executeRate float64,
) {
	dynamics.stamps = append(dynamics.stamps, float64(at.UnixNano()))

	dynamics.reynoldsHistory = appendFinite(dynamics.reynoldsHistory, reynolds, false)
	dynamics.divergenceHistory = appendFinite(dynamics.divergenceHistory, divergence, true)
	dynamics.viscosityHistory = appendFinite(dynamics.viscosityHistory, viscosity, false)
	dynamics.vorticityHistory = appendFinite(dynamics.vorticityHistory, vorticity, true)
	dynamics.turbulenceHistory = appendFinite(dynamics.turbulenceHistory, turbulence, true)
	dynamics.sourceBalanceRatio = appendSourceBalance(dynamics.sourceBalanceRatio, addRate, executeRate)

	dynamics.trim()
}

/*
trim keeps each series to the window depth derived from the shared stamp trail.
*/
func (dynamics *fluidDynamics) trim() {
	_, keep, err := statistic.ResolveWindows(dynamics.stamps, 0, 0)

	if err != nil || keep <= 0 {
		return
	}

	tail := func(values []float64) []float64 {
		if keep >= len(values) {
			return values
		}

		return values[len(values)-keep:]
	}

	dynamics.stamps = tail(dynamics.stamps)
	dynamics.reynoldsHistory = tail(dynamics.reynoldsHistory)
	dynamics.divergenceHistory = tail(dynamics.divergenceHistory)
	dynamics.viscosityHistory = tail(dynamics.viscosityHistory)
	dynamics.vorticityHistory = tail(dynamics.vorticityHistory)
	dynamics.turbulenceHistory = tail(dynamics.turbulenceHistory)
	dynamics.sourceBalanceRatio = tail(dynamics.sourceBalanceRatio)
}

/*
icebergBalanceFloor reports the upper-quartile balance ratio. It fires on the
first observation (the quantile of one sample is that sample) — low-confidence
evidence, not a >=N warmup gate.
*/
func (dynamics *fluidDynamics) icebergBalanceFloor() (float64, bool) {
	if len(dynamics.sourceBalanceRatio) == 0 {
		return 0, false
	}

	return sampleQuantile(0.75, dynamics.sourceBalanceRatio), true
}

func (dynamics *fluidDynamics) icebergScore(addRate, executeRate float64) float64 {
	if addRate <= 0 || executeRate <= 0 {
		return 0
	}

	activity := addRate + executeRate

	if activity <= 0 {
		return 0
	}

	balanceRatio := 1 - math.Abs(addRate-executeRate)/activity
	floor, ready := dynamics.icebergBalanceFloor()

	if !ready || balanceRatio < floor {
		return 0
	}

	return math.Min(addRate, executeRate)
}

func appendFinite(values []float64, value float64, allowZero bool) []float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return values
	}

	if (allowZero && value < 0) || (!allowZero && value <= 0) {
		return values
	}

	return append(values, value)
}

func appendSourceBalance(values []float64, addRate, executeRate float64) []float64 {
	if addRate <= 0 || executeRate <= 0 {
		return values
	}

	activity := addRate + executeRate
	balanceRatio := 1 - math.Abs(addRate-executeRate)/activity

	return append(values, balanceRatio)
}
