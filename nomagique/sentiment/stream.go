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
			cohortCount := float64(len(m.Peers))
			m.SetMetric("cohort_member_count", data.NewMetric[float64](
				"cohort_member_count",
				data.UnitCount,
				data.TimescaleInstantaneous,
				cohortCount,
				cohortCount,
			).Write(cohortCount))
			m.SetMetric("valid_member_count", data.NewMetric[float64](
				"valid_member_count",
				data.UnitCount,
				data.TimescaleInstantaneous,
				cohortCount,
				cohortCount,
			).Write(valid))
			m.SetMetric("excluded_member_count", data.NewMetric[float64](
				"excluded_member_count",
				data.UnitCount,
				data.TimescaleInstantaneous,
				0.0,
				cohortCount,
			).Write(0.0))

			if valid > 0 {
				halfValid := valid / 2.0
				m.SetMetric("advance_count", data.NewMetric[float64](
					"advance_count",
					data.UnitCount,
					data.TimescaleInstantaneous,
					halfValid,
					halfValid,
				).Write(positive))
				m.SetMetric("decline_count", data.NewMetric[float64](
					"decline_count",
					data.UnitCount,
					data.TimescaleInstantaneous,
					halfValid,
					halfValid,
				).Write(negative))
				m.SetMetric("unchanged_count", data.NewMetric[float64](
					"unchanged_count",
					data.UnitCount,
					data.TimescaleInstantaneous,
					0.0,
					valid,
				).Write(zero))

				advanceFraction := positive / valid
				declineFraction := negative / valid
				unchangedFraction := zero / valid
				breadth := advanceFraction - declineFraction

				m.SetMetric("advance_fraction", data.NewMetric[float64](
					"advance_fraction",
					data.UnitRatio,
					data.TimescaleInstantaneous,
					0.5,
					0.5,
				).Write(advanceFraction))
				m.SetMetric("decline_fraction", data.NewMetric[float64](
					"decline_fraction",
					data.UnitRatio,
					data.TimescaleInstantaneous,
					0.5,
					0.5,
				).Write(declineFraction))
				m.SetMetric("unchanged_fraction", data.NewMetric[float64](
					"unchanged_fraction",
					data.UnitRatio,
					data.TimescaleInstantaneous,
					0.0,
					1.0,
				).Write(unchangedFraction))
				m.SetMetric("advance_decline_spread", data.NewMetric[float64](
					"advance_decline_spread",
					data.UnitRatio,
					data.TimescaleInstantaneous,
					0.0,
					1.0,
				).Write(breadth))
				m.SetMetric("breadth", data.NewMetric[float64](
					"breadth",
					data.UnitRatio,
					data.TimescaleInstantaneous,
					0.0,
					1.0,
				).Write(breadth))

				participation := (positive + negative) / valid
				m.SetMetric("directional_participation", data.NewMetric[float64](
					"directional_participation",
					data.UnitRatio,
					data.TimescaleInstantaneous,
					0.5,
					0.5,
				).Write(participation))

				dirConsensus := 0.0
				if positive+negative > 0 {
					dirConsensus = math.Abs(positive-negative) / (positive + negative)
				}
				m.SetMetric("directional_consensus", data.NewMetric[float64](
					"directional_consensus",
					data.UnitRatio,
					data.TimescaleInstantaneous,
					0.5,
					0.5,
				).Write(dirConsensus))
				m.SetMetric("directional_agreement", data.NewMetric[float64](
					"directional_agreement",
					data.UnitRatio,
					data.TimescaleInstantaneous,
					0.5,
					0.5,
				).Write(dirConsensus*participation))

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

				var sumAbs, sumSq float64
				for _, c := range absChanges {
					sumAbs += c
					sumSq += c * c
				}

				meanAbs := sumAbs / valid
				rms := math.Sqrt(sumSq / valid)
				returnScale := rms

				m.SetMetric("median_return", data.NewMetric[float64](
					"median_return",
					data.UnitLogReturn,
					data.TimescaleInstantaneous,
					0.0,
					returnScale,
				).Write(medianReturn))
				m.SetMetric("signed_median", data.NewMetric[float64](
					"signed_median",
					data.UnitLogReturn,
					data.TimescaleInstantaneous,
					0.0,
					returnScale,
				).Write(medianReturn))
				m.SetMetric("median_absolute_return", data.NewMetric[float64](
					"median_absolute_return",
					data.UnitLogReturn,
					data.TimescaleInstantaneous,
					0.0,
					returnScale,
				).Write(medianAbsReturn))
				m.SetMetric("largest_absolute_return", data.NewMetric[float64](
					"largest_absolute_return",
					data.UnitLogReturn,
					data.TimescaleInstantaneous,
					0.0,
					returnScale,
				).Write(maxAbsChange))
				m.SetMetric("median_asof_age_seconds", data.NewMetric[float64](
					"median_asof_age_seconds",
					data.UnitSecond,
					data.TimescaleInstantaneous,
					0.0,
					maxAsofAge,
				).Write(medianAsofAge))
				m.SetMetric("median_from_age_seconds", data.NewMetric[float64](
					"median_from_age_seconds",
					data.UnitSecond,
					data.TimescaleInstantaneous,
					0.0,
					maxAsofAge,
				).Write(medianFromAge))
				m.SetMetric("max_asof_age_seconds", data.NewMetric[float64](
					"max_asof_age_seconds",
					data.UnitSecond,
					data.TimescaleInstantaneous,
					0.0,
					maxAsofAge,
				).Write(maxAsofAge))

				m.SetMetric("mean_absolute_return", data.NewMetric[float64](
					"mean_absolute_return",
					data.UnitLogReturn,
					data.TimescaleInstantaneous,
					0.0,
					returnScale,
				).Write(meanAbs))
				m.SetMetric("rms_return", data.NewMetric[float64](
					"rms_return",
					data.UnitLogReturn,
					data.TimescaleInstantaneous,
					0.0,
					returnScale,
				).Write(rms))

				var sumMad, sumMagMad float64
				for _, c := range changes {
					sumMad += math.Abs(c - medianReturn)
				}
				for _, c := range absChanges {
					sumMagMad += math.Abs(c - medianAbsReturn)
				}

				m.SetMetric("return_mad", data.NewMetric[float64](
					"return_mad",
					data.UnitLogReturn,
					data.TimescaleInstantaneous,
					0.0,
					returnScale,
				).Write(sumMad/valid))
				m.SetMetric("magnitude_mad", data.NewMetric[float64](
					"magnitude_mad",
					data.UnitLogReturn,
					data.TimescaleInstantaneous,
					0.0,
					returnScale,
				).Write(sumMagMad/valid))

				q1Idx := len(changes) / 4
				q3Idx := (len(changes) * 3) / 4
				m.SetMetric("return_interquartile_range", data.NewMetric[float64](
					"return_interquartile_range",
					data.UnitLogReturn,
					data.TimescaleInstantaneous,
					0.0,
					returnScale,
				).Write(changes[q3Idx]-changes[q1Idx]))

				if sumAbs > 0 {
					m.SetMetric("largest_move_share", data.NewMetric[float64](
						"largest_move_share",
						data.UnitRatio,
						data.TimescaleInstantaneous,
						0.0,
						1.0,
					).Write(maxAbsChange/sumAbs))
				}

				if hasFocal {
					halfValid := valid / 2.0
					m.SetMetric("return", data.NewMetric[float64](
						"return",
						data.UnitLogReturn,
						data.TimescaleInstantaneous,
						0.0,
						returnScale,
					).Write(focalChange))
					m.SetMetric("absolute_return", data.NewMetric[float64](
						"absolute_return",
						data.UnitLogReturn,
						data.TimescaleInstantaneous,
						0.0,
						returnScale,
					).Write(math.Abs(focalChange)))

					m.SetMetric("same_direction_peer_count", data.NewMetric[float64](
						"same_direction_peer_count",
						data.UnitCount,
						data.TimescaleInstantaneous,
						halfValid,
						halfValid,
					).Write(sameDir))
					m.SetMetric("opposite_direction_peer_count", data.NewMetric[float64](
						"opposite_direction_peer_count",
						data.UnitCount,
						data.TimescaleInstantaneous,
						halfValid,
						halfValid,
					).Write(oppDir))
					m.SetMetric("zero_return_peer_count", data.NewMetric[float64](
						"zero_return_peer_count",
						data.UnitCount,
						data.TimescaleInstantaneous,
						0.0,
						valid,
					).Write(zeroDir))

					m.SetMetric("same_direction_peer_fraction", data.NewMetric[float64](
						"same_direction_peer_fraction",
						data.UnitRatio,
						data.TimescaleInstantaneous,
						0.5,
						0.5,
					).Write(sameDir/valid))
					m.SetMetric("opposite_direction_peer_fraction", data.NewMetric[float64](
						"opposite_direction_peer_fraction",
						data.UnitRatio,
						data.TimescaleInstantaneous,
						0.5,
						0.5,
					).Write(oppDir/valid))
					m.SetMetric("zero_return_peer_fraction", data.NewMetric[float64](
						"zero_return_peer_fraction",
						data.UnitRatio,
						data.TimescaleInstantaneous,
						0.0,
						1.0,
					).Write(zeroDir/valid))
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
