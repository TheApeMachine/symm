/*
Package correlation composes dependence estimates between asynchronously
sampled paths.

Every node here satisfies the closed Node contract Step(Number) Number and
owns its state privately as growable slices. There is no interned symbol
table, no shared frame, and no fixed sample clamp.
*/
package correlation

import (
	"math"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Observation is one timestamped point on a path.
*/
type Observation struct {
	Nanos int64
	Value types.Number
}

/*
Path is a stateful primitive retaining a sequence of timestamped
observations.

It is the temporal analogue of Store: Observe records a sample, and Step
returns the reduction over the retained window, or 0 when no Reduce slot is
configured so the path behaves as an algebraic sink inside a Split.

Horizon is the optional adaptive window bounding retention. When it is
omitted the path grows unbounded — the v2 mandate, since a fixed clamp was
exactly what silently truncated v1's evidence. Supplying a Horizon opts into
causal contraction: an ADWIN horizon sheds observations once the stream drifts,
so the path retains only what remains statistically relevant.

Degenerate behavior: an omitted Reduce contributes 0 (Law of Sinks); an
omitted Horizon imposes no bound.
*/
type Path struct {
	Horizon *adaptive.Window
	Reduce  types.Reduction

	observations []Observation
}

/*
Observe records one timestamped sample under the adaptive horizon.

An observation that regresses in event time is refused, because every return
interval derived from the path assumes ascending time. A repeated timestamp
restates the same instant and replaces the sample in place.
*/
func (path *Path) Observe(nanos int64, value types.Number) bool {
	if count := len(path.observations); count > 0 {
		latest := path.observations[count-1].Nanos

		if nanos < latest {
			return false
		}

		if nanos == latest {
			path.observations[count-1].Value = value
			path.retain(value)

			return true
		}
	}

	path.observations = append(path.observations, Observation{
		Nanos: nanos,
		Value: value,
	})

	path.retain(value)

	return true
}

/*
retain advances the adaptive horizon, if one is configured, and drops
observations that fall outside the emergent capacity. With no Horizon the
path grows unbounded.
*/
func (path *Path) retain(value types.Number) {
	if path.Horizon == nil {
		return
	}

	capacity := path.Horizon.Step(float64(value))

	if capacity < minimumPathObservations {
		capacity = minimumPathObservations
	}

	if excess := len(path.observations) - capacity; excess > 0 {
		path.observations = path.observations[excess:]
	}
}

/*
Step folds the retained path through the Reduce slot. An omitted Reduce
returns 0, so a Path placed in a Split records the sample without disturbing
the parallel sum.
*/
func (path *Path) Step(x types.Number) types.Number {
	if path.Reduce == nil {
		return 0
	}

	values := make([]types.Number, len(path.observations))

	for index, observation := range path.observations {
		values[index] = observation.Value
	}

	return path.Reduce(values)
}

// Observations exposes the retained sequence.
func (path *Path) Observations() []Observation { return path.observations }

// Len reports how many observations are retained.
func (path *Path) Len() int { return len(path.observations) }

/*
Span returns the first and last retained timestamps, reporting false when the
path holds nothing.
*/
func (path *Path) Span() (int64, int64, bool) {
	if len(path.observations) == 0 {
		return 0, 0, false
	}

	return path.observations[0].Nanos,
		path.observations[len(path.observations)-1].Nanos,
		true
}

/*
minimumPathObservations is the smallest retained path that yields a single
return interval. It is a structural floor of the difference operator, not a
tuning constant.
*/
const minimumPathObservations = 2

/*
Interval is one log-return realized over the half-open span (From, To].
*/
type Interval struct {
	From  int64
	To    int64
	Value types.Number
}

/*
Returns folds a path into its valid log-return intervals.

An interval is valid only where time strictly advances and both endpoints are
positive, so a non-monotonic timestamp or a non-positive value contributes no
return rather than a spurious one.
*/
func (path *Path) Returns(destination []Interval) []Interval {
	destination = destination[:0]

	for index := 1; index < len(path.observations); index++ {
		previous := path.observations[index-1]
		current := path.observations[index]

		if previous.Nanos >= current.Nanos ||
			previous.Value <= 0 || current.Value <= 0 {
			continue
		}

		destination = append(destination, Interval{
			From:  previous.Nanos,
			To:    current.Nanos,
			Value: types.Number(math.Log(float64(current.Value / previous.Value))),
		})
	}

	return destination
}

/*
Spacings folds a path into the intervals between consecutive observations in
nanoseconds. Reduced by a median, this is the path's emergent sampling
resolution, so a lag search never scans on an invented grid.
*/
func (path *Path) Spacings(destination []types.Number) []types.Number {
	destination = destination[:0]

	for index := 1; index < len(path.observations); index++ {
		delta := path.observations[index].Nanos - path.observations[index-1].Nanos

		if delta <= 0 {
			continue
		}

		destination = append(destination, types.Number(delta))
	}

	return destination
}

/*
Energies folds a path into its interval-normalized return energies r²/Δt.
Reduced by a median, this is the path's typical return-energy rate.
*/
func (path *Path) Energies(destination []types.Number) []types.Number {
	destination = destination[:0]

	for index := 1; index < len(path.observations); index++ {
		previous := path.observations[index-1]
		current := path.observations[index]

		if previous.Nanos >= current.Nanos ||
			previous.Value <= 0 || current.Value <= 0 {
			continue
		}

		elapsed := float64(current.Nanos-previous.Nanos) / NanosPerSecond

		if elapsed <= 0 {
			continue
		}

		value := math.Log(float64(current.Value / previous.Value))
		destination = append(destination, types.Number(value*value/elapsed))
	}

	return destination
}

// NanosPerSecond is the nanosecond/second unit conversion.
const NanosPerSecond = 1e9

var _ types.Node = (*Path)(nil)
