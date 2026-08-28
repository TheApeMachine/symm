/*
Package recurrence implements bounded, causal historical-trajectory
comparison: given several already-standardized retained temporal.Path series
sharing one Frame, it answers whether the most recently observed window of
their joint trajectory resembles an earlier, non-overlapping window of the
same retained history.

This is the multivariate matrix-profile self-join signal/README.md §10
describes: a standardized trajectory Z_t compared against non-overlapping
historical subsequences, reporting distance and its causal percentile — never
a regime label, never a probability.

The dimensions a caller composes (flow, book, arrival-process) arrive on
independent clocks and at independent rates, so a joint trajectory must be
assembled in wall-clock time, not by sample ordinal. Analogue takes the
comparison horizon as an explicit Frame control fact — an absolute duration Q
in seconds — and treats every dimension as a piecewise-constant step function
of time (a standardized level holds until its next observation). The distance
between two windows is then the exact time-weighted RMS of their squared
difference, integrated over the window's actual change points — no invented
resampling grid, no samples compared by ordinal, and no value fabricated for a
period a dimension did not actually observe.

Recurrence itself never knows what produced its clock; whoever supplies Q is
responsible for its mathematical meaning.
*/
package recurrence

import (
	"math"
	"sort"

	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolDistance      = types.MustIntern("recurrence/nearest_distance")
	SymbolPercentile    = types.MustIntern("recurrence/nearest_percentile")
	SymbolMatchCount    = types.MustIntern("recurrence/match_count")
	SymbolMatchFromSec  = types.MustIntern("recurrence/match_from_unix_sec")
	SymbolMatchFromNsec = types.MustIntern("recurrence/match_from_unix_nsec")
	SymbolQueryLength   = types.MustIntern("recurrence/query_length")
	SymbolMaturity      = types.MustIntern("recurrence/maturity")

	// SymbolHorizon names the comparison-horizon control fact Analogue reads
	// from its input Frame: the absolute duration Q, in seconds, of one query
	// window. It must be positive and finite. It is a plain control slot, not a
	// statistic, so Analogue never derives it — the caller wires it from its own
	// characteristic timescale.
	SymbolHorizon = types.MustIntern("recurrence/horizon_sec")
)

// nanosecondsPerSecond is the fixed unit conversion for decoding the
// nanosecond-precision Unix timestamp a temporal.Path retains.
const nanosecondsPerSecond = int64(1_000_000_000)

/*
baselineCapacity bounds how many prior nearest distances the recurrence
percentile ranks against. It is the resident-state bound of the percentile's
own effective support — the same class of bound as temporal.MaxPathSamples —
not a statistical horizon and not derived from data: a fixed ring keeps the
baseline bounded and causal while its support fills toward the bound as history
accumulates. Below this bound the support grows one prior scan per step; at the
bound it becomes a causal sliding window, dropping the oldest scan first.
*/
const baselineCapacity = 16

// baselineSeries names the dedicated namespaced slots that carry the
// per-symbol ring of prior nearest distances. It is resolved once at wiring
// time; the slots live in the committed Frame, so the baseline is isolated per
// subject exactly like every other retained state.
var baselineSeries = temporal.NewSeries("recurrence/baseline")

/*
Analogue returns the primitive that performs one causal, bounded matrix-profile
self-join across the named series' retained joint trajectory, aligned in
wall-clock time over the absolute horizon Q read from SymbolHorizon each step.

Every series must already be a temporal.Path under its own prefix, and every
dimension is expected to already be standardized (a z-score or similarly
dimensionless, comparable quantity) by its own upstream signal.

Alignment is exact, not resampled: each dimension is a piecewise-constant step
function of wall-clock time, holding its last observed value until its next
observation. Two windows are compared by integrating the squared difference of
their joint step functions over the window's actual change points and dividing
by elapsed time — the time-weighted RMS. A period a dimension did not observe
is not compared at all: a window is only valid where every dimension is
defined, so no fabricated zero ever enters the distance.

The query window is [now-Q, now] ending at the most recent joint observation.
Candidate windows are every earlier non-overlapping interval of the same
duration Q, stepped back through the entire retained joint history until a
window would begin before any dimension's oldest retained observation. All
retained history is therefore searched, so the nearest analogue — however far
back it occurred — is always reachable, and match_count grows with retained
history instead of being capped at a fixed small number.

The percentile is a genuine causal percentile of today's nearest distance
against the bounded history of prior scans' nearest distances: the fraction of
prior nearest distances that are strictly closer than today's. Near 0 means
today's nearest match is unusually close (a recurring, familiar trajectory);
near 1 means it is unusually far (a novel trajectory). Ties do not inflate it,
and its support grows with history up to baselineCapacity.

Maturity follows the spec definition (signal/README.md §8) applied to this
scan's own effective support, the candidate count actually compared: 0 when at
most one candidate was searched, 1 - 1/match_count otherwise.

Output is explicitly undefined (no slot written) until Q is positive, the
query window is fully defined on every dimension, and at least one fully
defined candidate window exists entirely within retained history.
*/
func Analogue(prefixes ...string) types.Primitive {
	series := make([]temporal.Series, len(prefixes))

	for index, prefix := range prefixes {
		series[index] = temporal.NewSeries(prefix)
	}

	return func(input types.Frame) types.Frame {
		if len(series) == 0 {
			input.Err = types.PrimitiveError("recurrence: analogue requires at least one series")

			return input
		}

		horizonSeconds, hasHorizon := input.Get(SymbolHorizon)

		if !hasHorizon {
			// The control fact has not arrived yet (its signal has not
			// published a timescale). That is an absence, not a defect: the
			// output stays undefined for this step.
			return input
		}

		if horizonSeconds <= 0 || math.IsNaN(horizonSeconds) || math.IsInf(horizonSeconds, 0) {
			input.Err = types.PrimitiveError("recurrence: analogue requires a positive finite horizon")

			return input
		}

		horizonNanos := int64(horizonSeconds * float64(nanosecondsPerSecond))

		nowNanos, ok := latestTimestamp(series, &input)

		if !ok {
			return input
		}

		oldestNanos, ok := oldestTimestamp(series, &input)

		if !ok {
			return input
		}

		queryStart := nowNanos - horizonNanos

		if queryStart < oldestNanos {
			// The query window opens before the earliest joint observation:
			// the joint trajectory is not yet one full horizon long.
			return input
		}

		query, ok := stepWindow(series, &input, queryStart, nowNanos)

		if !ok {
			return input
		}

		// Sweep every earlier non-overlapping window of duration Q through the
		// whole retained joint history. A window is valid only while it lies
		// entirely at or after the earliest joint observation (each dimension
		// fully defined), so the sweep naturally stops at the true start of
		// history rather than at an arbitrary fixed count.
		nearestDistance := math.Inf(1)
		nearestStart := int64(0)
		candidateCount := 0

		for candidateEnd := queryStart; candidateEnd-horizonNanos >= oldestNanos; candidateEnd -= horizonNanos {
			candidateStart := candidateEnd - horizonNanos

			candidate, candidateOK := stepWindow(series, &input, candidateStart, candidateEnd)

			if !candidateOK {
				break
			}

			distance := timeWeightedDistance(query, candidate)
			candidateCount++

			if distance < nearestDistance {
				nearestDistance = distance
				nearestStart = candidateStart
			}
		}

		if candidateCount == 0 {
			return input
		}

		matchSec := nearestStart / nanosecondsPerSecond
		matchNsec := nearestStart % nanosecondsPerSecond

		// The percentile ranks today's nearest distance against the bounded
		// causal history of prior scans' nearest distances before appending
		// today's, so today never ranks against itself.
		percentile, ok := percentileOf(&input, nearestDistance)

		if !ok {
			// No prior scan yet: the distance is still defined, but there is
			// no baseline to rank it against, so the percentile is undefined.
			percentile = -1
		}

		appendBaseline(&input, nearestDistance)

		maturity := 0.0

		if candidateCount > 1 {
			maturity = 1 - 1/float64(candidateCount)
		}

		input.Put(SymbolDistance, nearestDistance)
		input.Put(SymbolMatchCount, float64(candidateCount))
		input.Put(SymbolMatchFromSec, float64(matchSec))
		input.Put(SymbolMatchFromNsec, float64(matchNsec))
		input.Put(SymbolQueryLength, horizonSeconds)
		input.Put(SymbolMaturity, maturity)

		if percentile >= 0 {
			input.Put(SymbolPercentile, percentile)
		}

		return input
	}
}

/*
stepWindow rebuilds one joint piecewise-constant trajectory over the interval
[start, end]: the merged, sorted change points of every series inside the
window, each carrying the joint vector of every dimension's held value at that
moment. The first point is always start (so the leading sub-interval from start
to the first real change is represented), and the last is end. ok is false only
when an internal read of a series' own retained sample fails.
*/
func stepWindow(
	series []temporal.Series,
	frame *types.Frame,
	start int64,
	end int64,
) ([]stepPoint, bool) {
	if end <= start {
		return nil, false
	}

	dimensions := len(series)
	changeTimes := []int64{start, end}

	for _, oneSeries := range series {
		count := oneSeries.Count(*frame)

		for index := 0; index < count; index++ {
			timestamp, _, found := oneSeries.Sample(frame, index)

			if !found {
				return nil, false
			}

			if timestamp > start && timestamp < end {
				changeTimes = append(changeTimes, timestamp)
			}
		}
	}

	sort.Slice(changeTimes, func(left, right int) bool {
		return changeTimes[left] < changeTimes[right]
	})

	// Deduplicate change times so equal timestamps across dimensions collapse
	// to one boundary with one held-value vector.
	unique := changeTimes[:0]
	previous := int64(-1)

	for _, changeTime := range changeTimes {
		if changeTime != previous {
			unique = append(unique, changeTime)
			previous = changeTime
		}
	}

	points := make([]stepPoint, len(unique))

	for index, changeTime := range unique {
		values := make([]float64, dimensions)

		for seriesIndex, oneSeries := range series {
			value, found := valueAtOrBefore(oneSeries, frame, changeTime)

			if !found {
				return nil, false
			}

			values[seriesIndex] = value
		}

		points[index] = stepPoint{at: changeTime, values: values}
	}

	return points, true
}

/*
stepPoint is one wall-clock instant at which the joint trajectory may change:
the time and the held value vector of every dimension immediately at that
instant. Consecutive stepPoints bound one sub-interval over which every
dimension is constant.
*/
type stepPoint struct {
	at     int64
	values []float64
}

/*
timeWeightedDistance computes the exact time-weighted RMS distance between two
joint piecewise-constant windows that share the same change-point grid (both
were built over the same [start, end] by stepWindow, so their grids coincide).
Each grid point's held-vector difference contributes its squared magnitude
weighted by the duration until the next point, summed across dimensions and
divided by (elapsed time × dimensions) — the square root is the RMS. A longer
duration of identical mismatch no longer inflates the distance beyond what the
RMS definition dictates, because elapsed time normalizes it.
*/
func timeWeightedDistance(query []stepPoint, candidate []stepPoint) float64 {
	if len(query) != len(candidate) || len(query) < 2 {
		return math.Inf(1)
	}

	dimensions := len(query[0].values)
	sumSquares := 0.0
	totalWidth := int64(0)

	for index := 0; index+1 < len(query); index++ {
		width := query[index+1].at - query[index].at

		if width <= 0 {
			continue
		}

		totalWidth += width

		for dimension := 0; dimension < dimensions; dimension++ {
			difference := query[index].values[dimension] - candidate[index].values[dimension]

			sumSquares += difference * difference * float64(width)
		}
	}

	if totalWidth <= 0 || dimensions == 0 {
		return math.Inf(1)
	}

	return math.Sqrt(sumSquares / (float64(totalWidth) * float64(dimensions)))
}

/*
latestTimestamp returns the most recent retained timestamp across every
series, which ends the query window.
*/
func latestTimestamp(series []temporal.Series, frame *types.Frame) (int64, bool) {
	latest := int64(0)
	found := false

	for _, oneSeries := range series {
		count := oneSeries.Count(*frame)

		if count == 0 {
			continue
		}

		timestamp, _, ok := oneSeries.Sample(frame, count-1)

		if !ok {
			continue
		}

		if !found || timestamp > latest {
			latest = timestamp
			found = true
		}
	}

	return latest, found
}

/*
oldestTimestamp returns the earliest retained timestamp across every series,
which is where the joint history begins. A window is fully defined only where
every dimension has observed, so comparisons stop before this point.
*/
func oldestTimestamp(series []temporal.Series, frame *types.Frame) (int64, bool) {
	oldest := int64(0)
	found := false

	for _, oneSeries := range series {
		count := oneSeries.Count(*frame)

		if count == 0 {
			continue
		}

		timestamp, _, ok := oneSeries.Sample(frame, 0)

		if !ok {
			continue
		}

		if !found || timestamp < oldest {
			oldest = timestamp
			found = true
		}
	}

	return oldest, found
}

/*
valueAtOrBefore returns the most recent retained value of the series observed
no later than the target time, scanning backward from the newest sample.
*/
func valueAtOrBefore(
	series temporal.Series,
	frame *types.Frame,
	target int64,
) (float64, bool) {
	count := series.Count(*frame)

	for index := count - 1; index >= 0; index-- {
		timestamp, value, ok := series.Sample(frame, index)

		if !ok {
			continue
		}

		if timestamp <= target {
			return value, true
		}
	}

	return 0, false
}

/*
percentileOf ranks one nearest distance against the bounded causal history of
prior nearest distances already retained in the committed Frame. It returns the
fraction of prior distances strictly closer (smaller) than the given distance —
the empirical novelty rank of today's nearest match — and ok=false when no
baseline exists yet. A value near 0 means today's nearest match is unusually
close (familiar, a recurring trajectory); near 1 means it is unusually far
(novel, an outlier). Ties contribute nothing, so two equally close prior scans
are not counted twice, and perfect (zero) recurrence saturates at 0 rather than
collapsing into maximal novelty.
*/
func percentileOf(frame *types.Frame, nearestDistance float64) (float64, bool) {
	count := baselineSeries.Count(*frame)

	if count == 0 {
		return 0, false
	}

	closer := 0

	for index := 0; index < count; index++ {
		value, found := baselineSeries.SampleAt(frame, index)

		if !found {
			continue
		}

		if value < nearestDistance {
			closer++
		}
	}

	return float64(closer) / float64(count), true
}

/*
appendBaseline writes one nearest distance into the per-symbol baseline ring,
evicting the oldest when the ring is full. The ring slot layout reuses the
series' own sample slots, holding one value per slot (no timestamps), so the
baseline is a plain bounded value ring in the committed Frame.
*/
func appendBaseline(frame *types.Frame, nearestDistance float64) {
	count := baselineSeries.Count(*frame)

	if count < baselineCapacity {
		frame.Put(baselineSeries.SampleSymbol(count), nearestDistance)
		frame.Put(baselineSeries.CountSymbol, float64(count+1))

		return
	}

	head := baselineSeries.Head(*frame)

	frame.Put(baselineSeries.SampleSymbol(head), nearestDistance)
	frame.Put(baselineSeries.HeadSymbol, float64((head+1)%baselineCapacity))
}
