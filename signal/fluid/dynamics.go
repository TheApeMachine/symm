package fluid

import (
	"math"
	"time"

	"github.com/theapemachine/nomagique/statistic"
)

/*
fluidDynamics holds the per-symbol rolling baselines the state evidence scales
against. Every series is recorded together once per Reading, so a single stamp
trail drives retention: the window depth is derived from observed event cadence
through nomagique statistic windows rather than a fixed ring capacity, and the
first observation already participates in scoring (no warmup sample gate).
*/
type fluidDynamics struct {
	stamps                   []float64
	reynoldsHistory          []float64
	divergenceHistory        []float64
	viscosityHistory         []float64
	velocityCurvatureHistory []float64
	turbulenceHistory        []float64
}

/*
record appends one coherent dynamics row stamped at the event time, then trims
every series to the cadence-derived window depth. Series whose value failed its
finiteness guard append NaN-free by carrying their prior length forward via the
caller's guards; callers pass already-validated values.
*/
func (dynamics *fluidDynamics) record(
	at time.Time,
	reynolds, divergence, viscosity, velocityCurvature, turbulence float64,
) {
	dynamics.stamps = append(dynamics.stamps, float64(at.UnixNano()))

	dynamics.reynoldsHistory = appendFinite(dynamics.reynoldsHistory, reynolds, true)
	dynamics.divergenceHistory = appendFinite(dynamics.divergenceHistory, divergence, true)
	dynamics.viscosityHistory = appendFinite(dynamics.viscosityHistory, viscosity, false)
	dynamics.velocityCurvatureHistory = appendFinite(
		dynamics.velocityCurvatureHistory,
		velocityCurvature,
		true,
	)
	dynamics.turbulenceHistory = appendFinite(dynamics.turbulenceHistory, turbulence, true)

	dynamics.trim()
}

/*
earliestStamp returns the minimum event time retained in the dynamics trail.
Out-of-order book frames can leave stamps unsorted, so history provenance must
scan the trail instead of assuming stamps[0] is the window origin.
*/
func (dynamics *fluidDynamics) earliestStamp() time.Time {
	if len(dynamics.stamps) == 0 {
		return time.Time{}
	}

	earliest := time.Unix(0, int64(dynamics.stamps[0])).UTC()

	for _, stamp := range dynamics.stamps[1:] {
		at := time.Unix(0, int64(stamp)).UTC()

		if at.Before(earliest) {
			earliest = at
		}
	}

	return earliest
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
	dynamics.velocityCurvatureHistory = tail(dynamics.velocityCurvatureHistory)
	dynamics.turbulenceHistory = tail(dynamics.turbulenceHistory)
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
