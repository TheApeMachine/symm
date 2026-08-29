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
percentile ranks against. It is the engine's own fixed-sample ceiling —
temporal.MaxPathSamples — not an independent constant and not a statistical
horizon: the baseline ring is the same kind of bounded retained series as every
temporal.Path, so it shares the same principled capacity. The percentile's
effective support fills toward this bound as history accumulates; at the bound
the ring becomes a causal sliding window, dropping the oldest scan first. It is
read from MaxPathSamples rather than hardcoded so the two resident-state bounds
can never silently drift apart.
*/
var baselineCapacity = temporal.MaxPathSamples

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

	return func(input *types.Frame) {
		if len(series) == 0 {
			input.Err = types.PrimitiveError("recurrence: analogue requires at least one series")

			return
		}

		horizonSeconds, hasHorizon := input.Get(SymbolHorizon)

		if !hasHorizon {
			// The control fact has not arrived yet (its signal has not
			// published a timescale). That is an absence, not a defect: the
			// output stays undefined for this step.
			return
		}

		if horizonSeconds <= 0 || math.IsNaN(horizonSeconds) || math.IsInf(horizonSeconds, 0) {
			input.Err = types.PrimitiveError("recurrence: analogue requires a positive finite horizon")

			return
		}

		horizonNanos := int64(horizonSeconds * float64(nanosecondsPerSecond))

		nowNanos, ok := latestTimestamp(series, input)

		if !ok {
			return
		}

		oldestNanos, ok := oldestTimestamp(series, input)

		if !ok {
			return
		}

		queryStart := nowNanos - horizonNanos

		if queryStart < oldestNanos {
			// The query window opens before the earliest joint observation:
			// the joint trajectory is not yet one full horizon long.
			return
		}

		query, ok := stepWindow(series, input, queryStart, nowNanos)

		if !ok {
			return
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

			candidate, candidateOK := stepWindow(series, input, candidateStart, candidateEnd)

			if !candidateOK {
				break
			}

			distance := timeWeightedDistance(query, candidate, horizonNanos)
			candidateCount++

			if distance < nearestDistance {
				nearestDistance = distance
				nearestStart = candidateStart
			}
		}

		if candidateCount == 0 {
			return
		}

		matchSec := nearestStart / nanosecondsPerSecond
		matchNsec := nearestStart % nanosecondsPerSecond

		// The percentile ranks today's nearest distance against the bounded
		// causal history of prior scans' nearest distances before appending
		// today's, so today never ranks against itself.
		percentile, ok := percentileOf(input, nearestDistance)

		if !ok {
			// No prior scan yet: the distance is still defined, but there is
			// no baseline to rank it against, so the percentile is undefined.
			percentile = -1
		}

		appendBaseline(input, nearestDistance)

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
	}
}

/*
stepWindow rebuilds one window as, per dimension, its piecewise-constant step
function over the interval [start, end], expressed in relative offsets from the
window's own start. Each dimension's segments are sorted by offset; the first
segment of every dimension begins at offset 0 (the value held at the window's
start) and the last extends to the window's end. Relative offsets — not
absolute timestamps — are what make two windows comparable: the query and every
candidate all span exactly the same duration, so offset 0..Q is the shared
coordinate frame in which their (independently irregular) change patterns can
be merged. ok is false only when an internal read of a series' own retained
sample fails.
*/
func stepWindow(
	series []temporal.Series,
	frame *types.Frame,
	start int64,
	end int64,
) ([]dimensionStep, bool) {
	if end <= start {
		return nil, false
	}

	dimensions := len(series)
	steps := make([]dimensionStep, dimensions)

	for seriesIndex, oneSeries := range series {
		// The leading value is whatever the dimension held at the window's
		// start (its most recent observation at or before start).
		leadingValue, found := valueAtOrBefore(oneSeries, frame, start)

		if !found {
			return nil, false
		}

		segments := []stepAt{{offset: 0, value: leadingValue}}

		count := oneSeries.Count(*frame)

		for index := 0; index < count; index++ {
			timestamp, value, ok := oneSeries.Sample(frame, index)

			if !ok {
				return nil, false
			}

			if timestamp > start && timestamp < end {
				segments = append(segments, stepAt{offset: timestamp - start, value: value})
			}
		}

		// Samples are already chronological, and the leading segment is at
		// offset 0, so segments are naturally sorted by offset.
		steps[seriesIndex] = dimensionStep{segments: segments}
	}

	return steps, true
}

/*
dimensionStep is one dimension's piecewise-constant trajectory inside a window:
an ordered list of (offset, value) thresholds. The value holds from its own
offset until the next threshold's offset — or the window's end for the last.
*/
type dimensionStep struct {
	segments []stepAt
}

/*
stepAt is one threshold of a dimension's step function: the relative offset at
which the dimension changes and the value it holds from that offset onward.
*/
type stepAt struct {
	offset int64
	value  float64
}

/*
timeWeightedDistance computes the exact time-weighted RMS distance between two
windows whose change grids are independent. It merges the two windows' relative
change offsets into one shared grid, holds each window's value per dimension
across every resulting interval, and integrates the squared difference weighted
by interval width, divided by (window duration × dimensions) under the square
root. Both windows span the same duration — passed in explicitly — so their
relative offsets coincide in a shared coordinate frame even when their change
patterns differ, and the distance is exact for arbitrary asynchronous
observations.
*/
func timeWeightedDistance(query []dimensionStep, candidate []dimensionStep, duration int64) float64 {
	if len(query) != len(candidate) || len(query) == 0 {
		return math.Inf(1)
	}

	if duration <= 0 {
		return math.Inf(1)
	}

	dimensions := len(query)
	offsets := mergedOffsets(query, candidate, duration)
	sumSquares := 0.0

	for index := 0; index+1 < len(offsets); index++ {
		width := offsets[index+1] - offsets[index]

		if width <= 0 {
			continue
		}

		for dimension := 0; dimension < dimensions; dimension++ {
			queryValue := heldValue(query[dimension].segments, offsets[index])
			candidateValue := heldValue(candidate[dimension].segments, offsets[index])

			difference := queryValue - candidateValue

			sumSquares += difference * difference * float64(width)
		}
	}

	return math.Sqrt(sumSquares / (float64(duration) * float64(dimensions)))
}

/*
mergedOffsets returns the sorted, deduplicated union of both windows' relative
change offsets plus the shared start (0) and end offsets, so every sub-interval
over which both step functions are constant is enumerated exactly once.
*/
func mergedOffsets(query []dimensionStep, candidate []dimensionStep, end int64) []int64 {
	totalSegments := 2 // the shared start (0) and end offsets

	for _, window := range [][]dimensionStep{query, candidate} {
		for _, dimension := range window {
			totalSegments += len(dimension.segments)
		}
	}

	// A sort-then-dedup slice replaces the map: this runs once per candidate
	// pair inside the analogue search, so a map's per-entry bucket overhead
	// (versus a contiguous slice) was a direct, avoidable allocation cost on
	// every ticker tick.
	offsets := make([]int64, 0, totalSegments)
	offsets = append(offsets, 0, end)

	for _, window := range [][]dimensionStep{query, candidate} {
		for _, dimension := range window {
			for _, segment := range dimension.segments {
				offsets = append(offsets, segment.offset)
			}
		}
	}

	sort.Slice(offsets, func(left, right int) bool {
		return offsets[left] < offsets[right]
	})

	deduped := offsets[:0]
	var previous int64
	hasPrevious := false

	for _, offset := range offsets {
		if hasPrevious && offset == previous {
			continue
		}

		deduped = append(deduped, offset)
		previous = offset
		hasPrevious = true
	}

	return deduped
}

/*
heldValue returns the value a dimension holds at a given relative offset: the
value of the last threshold whose offset is at or before the queried offset.
*/
func heldValue(segments []stepAt, offset int64) float64 {
	value := 0.0

	for _, segment := range segments {
		if segment.offset > offset {
			break
		}

		value = segment.value
	}

	return value
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
