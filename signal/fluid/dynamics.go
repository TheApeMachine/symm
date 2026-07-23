package fluid

import (
	"fmt"
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
) error {
	if at.IsZero() {
		return fmt.Errorf("fluid: dynamics observation time required")
	}

	values := []struct {
		name         string
		value        float64
		positiveOnly bool
	}{
		{"reynolds", reynolds, false},
		{"divergence", divergence, false},
		{"viscosity", viscosity, true},
		{"velocity curvature", velocityCurvature, false},
		{"turbulence", turbulence, false},
	}

	for _, observed := range values {
		if math.IsNaN(observed.value) || math.IsInf(observed.value, 0) ||
			observed.value < 0 || observed.positiveOnly && observed.value == 0 {
			return fmt.Errorf("fluid: %s dynamics value is invalid", observed.name)
		}
	}

	dynamics.stamps = append(dynamics.stamps, float64(at.UnixNano()))
	dynamics.reynoldsHistory = append(dynamics.reynoldsHistory, reynolds)
	dynamics.divergenceHistory = append(dynamics.divergenceHistory, divergence)
	dynamics.viscosityHistory = append(dynamics.viscosityHistory, viscosity)
	dynamics.velocityCurvatureHistory = append(
		dynamics.velocityCurvatureHistory,
		velocityCurvature,
	)
	dynamics.turbulenceHistory = append(dynamics.turbulenceHistory, turbulence)

	return dynamics.trim()
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
func (dynamics *fluidDynamics) trim() error {
	_, keep, err := statistic.ResolveWindows(dynamics.stamps, 0, 0)

	if err != nil {
		return fmt.Errorf("fluid: resolve dynamics retention: %w", err)
	}

	if keep <= 0 {
		return fmt.Errorf("fluid: dynamics retention must be positive")
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

	return nil
}
