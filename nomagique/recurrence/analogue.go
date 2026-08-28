/*
Package recurrence implements bounded, causal historical-trajectory
comparison: given several already-standardized retained temporal.Path series
sharing one Frame, it answers whether the most recently observed segment of
their joint trajectory resembles an earlier, non-overlapping segment of the
same retained history.

This is the multivariate matrix-profile self-join signal/README.md §10
describes: a standardized trajectory Z_t compared against non-overlapping
historical subsequences, reporting distance and its within-scan percentile —
never a regime label, never a probability.
*/
package recurrence

import (
	"math"

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
)

/*
minimumQueryLength is the smallest subsequence that has a shape at all: one
point has a value but no trajectory, so a query needs at least two to admit a
notion of distance between paths. This is a definitional floor of "what is a
trajectory", not a tunable horizon — it never changes and is not derived from
data because two is the smallest count for which a difference exists.
*/
const minimumQueryLength = 2

// nanosecondsPerSecond is the fixed unit conversion for decoding the
// nanosecond-precision Unix timestamp Path retains, not a tunable horizon.
const nanosecondsPerSecond = int64(1_000_000_000)

/*
Analogue returns the primitive that performs one causal, bounded matrix-profile
self-join across the named series' retained joint trajectory. Every series
must already be a temporal.Path under its own prefix, and every dimension is
expected to already be standardized (a z-score or similarly dimensionless,
comparable quantity) by its own upstream signal, so the distance is a plain
multivariate Euclidean distance across dimensions with no additional
normalization performed here — normalizing twice would silently change what
"close" means without the caller's knowledge.

The comparison horizon is derived entirely from how much joint history has
actually accumulated, never a constant: sampleCount is the smallest retained
count across every dimension (the joint trajectory is only as long as its
least-observed dimension), and a candidate is only compared once a full
query-length gap separates it from the query — the standard matrix-profile
self-match exclusion, itself a fraction of the query length and therefore
adaptive along with it, never an independent constant.

The query length is queryLength = 2*sampleCount/5. This is not an arbitrary
split: with the exclusion radius fixed at half the query length (the standard
definition), placing one query, one exclusion gap, and at least one candidate
of the same length back-to-back consumes 2.5×queryLength samples in the worst
case, so queryLength = sampleCount/2.5 = 2*sampleCount/5 is the tightest
fraction of the retained trajectory that structurally guarantees room for at
least one non-overlapping candidate as soon as one exists at all — any larger
query would mathematically leave no room for a candidate no matter how much
more history accumulated, and any smaller query discards evidence the data
already earned.

Every candidate distance the scan actually computes contributes to the
reported percentile, so match_count and the percentile are always mutually
consistent with what was searched — never a hidden or partial scan.

Maturity follows the spec definition (signal/README.md §8) applied to this
scan's own effective support, the candidate count: 0 when at most one
candidate was searched, 1 - 1/candidateCount otherwise. This is the honest
quality of the comparison itself — how much historical evidence the nearest
match was actually chosen from — not borrowed from any one input dimension's
own Measurement quality, which describes a different thing entirely.

Output is explicitly undefined (no slot written) until queryLength reaches
minimumQueryLength and at least one valid non-overlapping candidate exists.
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

		sampleCount := series[0].Count(input)

		for _, oneSeries := range series[1:] {
			if count := oneSeries.Count(input); count < sampleCount {
				sampleCount = count
			}
		}

		queryLength := (2 * sampleCount) / 5

		if queryLength < minimumQueryLength {
			return input
		}

		exclusionRadius := queryLength / 2
		queryStart := sampleCount - queryLength
		lastCandidateStart := queryStart - exclusionRadius - queryLength

		if lastCandidateStart < 0 {
			return input
		}

		nearestDistance := math.Inf(1)
		nearestStart := -1
		candidateCount := 0
		var distances [temporal.MaxPathSamples]float64

		for candidateStart := 0; candidateStart <= lastCandidateStart; candidateStart++ {
			distance, ok := windowDistance(&input, series, queryStart, candidateStart, queryLength)

			if !ok {
				input.Err = types.PrimitiveError("recurrence: retained path sample is missing inside its own count")

				return input
			}

			distances[candidateCount] = distance
			candidateCount++

			if distance < nearestDistance {
				nearestDistance = distance
				nearestStart = candidateStart
			}
		}

		if candidateCount == 0 {
			return input
		}

		rank := 0

		for _, distance := range distances[:candidateCount] {
			if distance <= nearestDistance {
				rank++
			}
		}

		matchTimestamp, _, found := series[0].Sample(&input, nearestStart)

		if !found {
			input.Err = types.PrimitiveError("recurrence: nearest match start sample is missing")

			return input
		}

		matchSec := matchTimestamp / nanosecondsPerSecond
		matchNsec := matchTimestamp % nanosecondsPerSecond

		maturity := 0.0

		if candidateCount > 1 {
			maturity = 1 - 1/float64(candidateCount)
		}

		input.Put(SymbolDistance, nearestDistance)
		input.Put(SymbolPercentile, float64(rank)/float64(candidateCount))
		input.Put(SymbolMatchCount, float64(candidateCount))
		input.Put(SymbolMatchFromSec, float64(matchSec))
		input.Put(SymbolMatchFromNsec, float64(matchNsec))
		input.Put(SymbolQueryLength, float64(queryLength))
		input.Put(SymbolMaturity, maturity)

		return input
	}
}

/*
windowDistance computes the multivariate Euclidean distance between the query
window and one candidate window of the same length, summed across every
dimension. Every dimension contributes the sum of squared differences of its
own already-standardized samples; ok is false only if a retained sample inside
the series' own reported count could not be read back, which is an internal
consistency failure rather than an absence.
*/
func windowDistance(
	frame *types.Frame,
	series []temporal.Series,
	queryStart int,
	candidateStart int,
	length int,
) (float64, bool) {
	sumSquares := 0.0

	for _, oneSeries := range series {
		for offset := 0; offset < length; offset++ {
			_, queryValue, queryFound := oneSeries.Sample(frame, queryStart+offset)
			_, candidateValue, candidateFound := oneSeries.Sample(frame, candidateStart+offset)

			if !queryFound || !candidateFound {
				return 0, false
			}

			difference := queryValue - candidateValue
			sumSquares += difference * difference
		}
	}

	return math.Sqrt(sumSquares), true
}
