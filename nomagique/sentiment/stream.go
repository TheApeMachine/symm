package sentiment

import (
	"errors"
	"iter"
	"math"
	"sort"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
CrossSentiment computes cross-sectional sentiment metrics across the active peer cohort.
It measures cohort breadth, dispersion, median returns, directional agreement,
and focal alignment without editorial classification.
*/
type CrossSentiment struct {
	err error
}

func NewCrossSentiment() core.Primitive {
	return &CrossSentiment{}
}

func (op *CrossSentiment) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m == nil || m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			if len(m.Peers) == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			positive, negative, zero := 0.0, 0.0, 0.0
			var extremeKey string
			var maxAbsChange float64

			var changes []float64
			var absChanges []float64

			var sameDir, oppDir, zeroDir float64
			var focalChange float64
			var hasFocal bool

			for _, peer := range m.Peers {
				if peer.Label == m.Label {
					focalChange = peer.GetMetric("change").Raw
					hasFocal = true
				}
			}

			var maxAsofAge float64
			var asofAges []float64
			var fromAges []float64

			for _, peer := range m.Peers {
				change := peer.GetMetric("change").Raw
				absChange := math.Abs(change)

				changes = append(changes, change)
				absChanges = append(absChanges, absChange)

				if absChange > maxAbsChange || extremeKey == "" {
					maxAbsChange = absChange
					extremeKey = peer.Label
				}

				if change > 0 {
					positive++
				}

				if change < 0 {
					negative++
				}

				if change == 0 {
					zero++
				}

				if hasFocal {
					if (change > 0 && focalChange > 0) || (change < 0 && focalChange < 0) {
						sameDir++
					}

					if (change > 0 && focalChange < 0) || (change < 0 && focalChange > 0) {
						oppDir++
					}

					if change == 0 || focalChange == 0 {
						zeroDir++
					}
				}

				asofAge := m.At.Sub(peer.At).Seconds()
				fromAge := m.At.Sub(peer.From).Seconds()

				if asofAge > maxAsofAge {
					maxAsofAge = asofAge
				}

				asofAges = append(asofAges, asofAge)
				fromAges = append(fromAges, fromAge)
			}

			valid := positive + negative + zero
			m.WriteMetric("cohort_member_count", float64(len(m.Peers)))
			m.WriteMetric("valid_member_count", valid)
			m.WriteMetric("excluded_member_count", 0.0)

			if valid > 0 {
				m.WriteMetric("advance_count", positive)
				m.WriteMetric("decline_count", negative)
				m.WriteMetric("unchanged_count", zero)

				advanceFraction := positive / valid
				declineFraction := negative / valid
				unchangedFraction := zero / valid
				breadth := advanceFraction - declineFraction

				m.WriteNormalized("advance_fraction", advanceFraction)
				m.WriteNormalized("decline_fraction", declineFraction)
				m.WriteNormalized("unchanged_fraction", unchangedFraction)
				m.WriteNormalized("advance_decline_spread", breadth)
				m.WriteNormalized("breadth", breadth)

				participation := (positive + negative) / valid
				m.WriteNormalized("directional_participation", participation)

				dirConsensus := 0.0
				if positive+negative > 0 {
					dirConsensus = math.Abs(positive-negative) / (positive + negative)
				}
				m.WriteNormalized("directional_consensus", dirConsensus)
				m.WriteNormalized("directional_agreement", dirConsensus*participation)

				if extremeKey != "" {
					m.EnsureMetadata()
					m.SetProvenance("extreme_key", extremeKey)
					m.SetProvenance("largest_move_symbol", extremeKey)
				}

				sort.Float64s(changes)
				sort.Float64s(absChanges)
				sort.Float64s(asofAges)
				sort.Float64s(fromAges)

				mid := len(changes) / 2
				var medianReturn, medianAbsReturn, medianAsofAge, medianFromAge float64
				if len(changes)%2 == 0 {
					medianReturn = (changes[mid-1] + changes[mid]) / 2.0
					medianAbsReturn = (absChanges[mid-1] + absChanges[mid]) / 2.0
					medianAsofAge = (asofAges[mid-1] + asofAges[mid]) / 2.0
					medianFromAge = (fromAges[mid-1] + fromAges[mid]) / 2.0
				}

				if len(changes)%2 != 0 {
					medianReturn = changes[mid]
					medianAbsReturn = absChanges[mid]
					medianAsofAge = asofAges[mid]
					medianFromAge = fromAges[mid]
				}

				m.WriteMetric("median_return", medianReturn)
				m.WriteMetric("signed_median", medianReturn)
				m.WriteMetric("median_absolute_return", medianAbsReturn)
				m.WriteMetric("largest_absolute_return", maxAbsChange)
				m.WriteMetric("median_asof_age_seconds", medianAsofAge)
				m.WriteMetric("median_from_age_seconds", medianFromAge)
				m.WriteMetric("max_asof_age_seconds", maxAsofAge)

				var sumAbs, sumSq float64
				for _, c := range absChanges {
					sumAbs += c
					sumSq += c * c
				}

				meanAbs := sumAbs / valid
				rms := math.Sqrt(sumSq / valid)
				m.WriteMetric("mean_absolute_return", meanAbs)
				m.WriteMetric("rms_return", rms)

				var sumMad, sumMagMad float64
				for _, c := range changes {
					sumMad += math.Abs(c - medianReturn)
				}
				for _, c := range absChanges {
					sumMagMad += math.Abs(c - medianAbsReturn)
				}

				m.WriteMetric("return_mad", sumMad/valid)
				m.WriteMetric("magnitude_mad", sumMagMad/valid)

				q1Idx := len(changes) / 4
				q3Idx := (len(changes) * 3) / 4
				m.WriteMetric("return_interquartile_range", changes[q3Idx]-changes[q1Idx])

				if sumAbs > 0 {
					m.WriteNormalized("largest_move_share", maxAbsChange/sumAbs)
				}

				if hasFocal {
					m.WriteMetric("return", focalChange)
					m.WriteMetric("absolute_return", math.Abs(focalChange))

					m.WriteMetric("same_direction_peer_count", sameDir)
					m.WriteMetric("opposite_direction_peer_count", oppDir)
					m.WriteMetric("zero_return_peer_count", zeroDir)

					m.WriteNormalized("same_direction_peer_fraction", sameDir/valid)
					m.WriteNormalized("opposite_direction_peer_fraction", oppDir/valid)
					m.WriteNormalized("zero_return_peer_fraction", zeroDir/valid)
				}
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *CrossSentiment) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
