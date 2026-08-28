/*
Package recurrence implements bounded, causal historical-trajectory
comparison: given several already-standardized retained temporal.Path series
sharing one Frame, it answers whether the most recently observed window of
their joint trajectory resembles an earlier, non-overlapping window of the
same retained history.

This is the multivariate matrix-profile self-join signal/README.md §10
describes: a standardized trajectory Z_t compared against non-overlapping
historical subsequences, reporting distance and its discord score — never a
regime label, never a probability.

The three dimensions a caller composes (flow, book, arrival-process) arrive on
independent clocks: different streams observe at different instants and at
different rates. A joint trajectory must therefore be assembled in wall-clock
time, not by sample ordinal. Analogue takes the comparison horizon as an
explicit Frame control fact — an absolute duration Q in seconds — selects, for
each series, the observations falling inside [now−Q, now], aligns them to one
shared clock by carrying the last value forward where a series is momentarily
quiet, and compares that aligned query against earlier aligned windows of the
same duration. Recurrence itself never knows what produced its clock; whoever
supplies Q is responsible for its mathematical meaning.
*/
package recurrence

import (
	"math"

	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolDistance      = types.MustIntern("recurrence/nearest_distance")
	SymbolPercentile    = types.MustIntern("recurrence/discord_score")
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
candidateWindows is the fixed number of earlier, non-overlapping aligned
windows the query is compared against on every scan. It is not a horizon and
is not derived from data: it is the width of the distribution the discord
score ranks the nearest match within. Two windows is the minimum that allows a
nearest match to be either unusually close (recurring) or unusually far
(novel) relative to the retained history, and keeps the scan bounded and
causal as history grows instead of recomputing over an ever-lengthening
backlog.
*/
const candidateWindows = 2

/*
alignedSteps is the fixed number of equal time steps one query window of
duration Q is resampled onto. It is a fixed structural subdivision of the
window — the "shape" resolution of a trajectory — not a tunable horizon, and
it is identical for the query and every candidate so distances compare
like-shaped windows. Independent of Q, so distances stay length-normalized
across differently-horizoned comparisons.
*/
const alignedSteps = 8

/*
Analogue returns the primitive that performs one causal, bounded matrix-profile
self-join across the named series' retained joint trajectory, aligned in
wall-clock time over the absolute horizon Q read from SymbolHorizon each step.

Every series must already be a temporal.Path under its own prefix, and every
dimension is expected to already be standardized (a z-score or similarly
dimensionless, comparable quantity) by its own upstream signal. Distance is a
length-normalized RMS across dimensions — the square root of the mean squared
per-step mismatch — so a longer window does not inflate distance merely because
it spans more steps, and comparisons across differing query lengths remain
comparable.

The comparison horizon is the caller-supplied duration Q, never a sample count
and never a constant of this package. A query window is the interval [now−Q,
now] ending at the most recent joint observation; each candidate window is one
of candidateWindows earlier, non-overlapping intervals of the same duration,
stepping Q+spacing earlier into history. Because the windows are fixed in
number and separated in time, the fraction of retained history actually
searched grows steadily with that history — effective support accrues the way
evidence is supposed to — instead of cycling on a sample-count remainder.

The percentile is the matrix-profile discord score M(x) = sqrt(nearest / mean)
over the candidate distances: a value near 0 means the nearest match is
unusually close (the trajectory recurs), a value near 1 means the nearest match
is an outlier (the trajectory is novel). It ranks the nearest distance against
the observed background of candidate distances, so a tie among several equally
close candidates no longer masquerades as recurrence.

Maturity follows the spec definition (signal/README.md §8) applied to this
scan's own effective support, the count of candidate windows actually
compared: 0 when at most one was searched, 1 - 1/candidateCount otherwise.

Output is explicitly undefined (no slot written) until Q is positive, the
query window contains at least two aligned steps on every dimension, and at
least one non-overlapping candidate window exists entirely within retained
history.
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

		queryStart := nowNanos - horizonNanos

		query, ok := alignedWindow(series, &input, queryStart, nowNanos)

		if !ok {
			return input
		}

		// Sweep candidate windows earlier in time. Each candidate occupies a
		// duration Q and is separated from the next window by a spacing so
		// windows never overlap; stepping into history by Q+spacing per window
		// guarantees causal non-overlap with the query and with each other.
		nearestDistance := math.Inf(1)
		nearestStart := int64(0)
		distanceSum := 0.0
		candidateCount := 0

		for windowIndex := 0; windowIndex < candidateWindows; windowIndex++ {
			candidateEnd := queryStart - int64(windowIndex)*(horizonNanos+alignmentSpacingNanos(horizonNanos))
			candidateStart := candidateEnd - horizonNanos

			candidate, candidateOK := alignedWindow(series, &input, candidateStart, candidateEnd)

			if !candidateOK {
				break
			}

			distance := rmsDistance(query, candidate)
			distanceSum += distance
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

		discord := discordScore(nearestDistance, distanceSum, candidateCount)

		maturity := 0.0

		if candidateCount > 1 {
			maturity = 1 - 1/float64(candidateCount)
		}

		input.Put(SymbolDistance, nearestDistance)
		input.Put(SymbolPercentile, discord)
		input.Put(SymbolMatchCount, float64(candidateCount))
		input.Put(SymbolMatchFromSec, float64(matchSec))
		input.Put(SymbolMatchFromNsec, float64(matchNsec))
		input.Put(SymbolQueryLength, horizonSeconds)
		input.Put(SymbolMaturity, maturity)

		return input
	}
}

/*
alignmentSpacingNanos is the non-overlap spacing between successive candidate
windows: a fixed fraction of the horizon Q, so every window is separated from
its neighbours in proportion to the timescale being compared. The query can
therefore never match itself or a trivially overlapping segment.
*/
func alignmentSpacingNanos(horizonNanos int64) int64 {
	return int64(float64(horizonNanos) * spacingFraction)
}

/*
spacingFraction is the fraction of Q left as a non-overlap buffer between
windows: one window, one buffer, and the query laid back-to-back fit neatly
into a compact causal layout without any count-derived arithmetic.
*/
const spacingFraction = 0.5

/*
latestTimestamp returns the most recent retained timestamp across every
series, which ends the query window. A series whose path is empty contributes
nothing; if every series is empty the query has no anchor and the output stays
undefined.
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
alignedWindow assembles one length-aligned joint observation of every series
over the interval [start, end]. The window is split into alignedSteps equal
time steps; at each step a series contributes its most recent value observed
no later than that step's time, carrying the previous value forward when the
series is quiet (a standardized level is held, not zero-filled — inventing a
zero would pretend the dimension vanished). The result is a flat slice of
dimension-major values: step 0 of every series, then step 1 of every series,
and so on. ok is false when any series has no retained value at or before the
window start, so the window would begin in the middle of an unobserved gap.
*/
func alignedWindow(
	series []temporal.Series,
	frame *types.Frame,
	start int64,
	end int64,
) ([]float64, bool) {
	dimensions := len(series)
	stepDuration := (end - start) / alignedSteps

	if stepDuration <= 0 {
		return nil, false
	}

	aligned := make([]float64, alignedSteps*dimensions)

	for seriesIndex, oneSeries := range series {
		carry := 0.0
		hasCarry := false

		for step := 0; step < alignedSteps; step++ {
			stepTime := start + int64(step+1)*stepDuration
			value, found := valueAtOrBefore(oneSeries, frame, stepTime)

			if found {
				carry = value
				hasCarry = true
			}

			aligned[seriesIndex+step*dimensions] = carry
		}

		if !hasCarry {
			// This dimension had no observation anywhere in the window: the
			// joint trajectory cannot be assembled and the output stays
			// undefined rather than fabricating a held zero.
			return nil, false
		}
	}

	return aligned, true
}

/*
valueAtOrBefore returns the most recent retained value of the series observed
no later than the target time. It scans backward from the newest sample, so
the freshest qualifying observation is returned without a full forward pass.
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
rmsDistance computes the length-normalized Euclidean distance between two
joint aligned windows: the square root of the mean over all dimensions and
steps of the squared per-step difference. Dividing by the total element count
makes the distance an RMS mismatch per component, so a longer window (more
steps) never inflates the distance merely by spanning more samples.
*/
func rmsDistance(query []float64, candidate []float64) float64 {
	if len(query) == 0 || len(query) != len(candidate) {
		return math.Inf(1)
	}

	sumSquares := 0.0

	for index := range query {
		difference := query[index] - candidate[index]

		sumSquares += difference * difference
	}

	return math.Sqrt(sumSquares / float64(len(query)))
}

/*
discordScore is the matrix-profile discord score: the square root of the
nearest distance divided by the mean candidate distance. Near 0 the nearest
match is unusually close (a recurring trajectory); near 1 the nearest match is
a far outlier (a novel trajectory). It is scale-free and unaffected by a tie
among several equally close candidates, so recurrence is measured by how the
nearest neighbour stands out from background, not by how many distances happen
to tie at the minimum.
*/
func discordScore(nearestDistance float64, distanceSum float64, candidateCount int) float64 {
	if candidateCount <= 0 {
		return 0
	}

	meanDistance := distanceSum / float64(candidateCount)

	if meanDistance <= 0 {
		return 0
	}

	score := math.Sqrt(nearestDistance / meanDistance)

	if score > 1 {
		return 1
	}

	return score
}
