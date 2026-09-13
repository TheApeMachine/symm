package sentiment

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Fold folds the focal symbol's last price into the shared cross-section and
projects the snapshot's cohort facts onto the measurement where they are
computed. Without a produced snapshot there is nothing to project and the
measurement moves through untouched.
*/
type Fold struct {
	err     error
	section core.Primitive
}

func NewFold() core.Primitive {
	return &Fold{section: data.NewCrossSection()}
}

func (op *Fold) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)
			last := m.Metrics["last"].Raw

			if m.Err != nil || last == 0 {
				if !yield(arriving) {
					return
				}

				continue
			}

			var snapshot data.Snapshot
			hasSnapshot := false

			for out := range op.section.Next(transport.NewValues(data.SectionInput{
				Key: m.Label, Value: last, At: m.At, Focal: m.Label,
			}).Next(nil)) {
				snapshot = *(*data.Snapshot)(out)
				hasSnapshot = true
			}

			if err := op.section.Error(); err != nil {
				m.Err = errors.Join(m.Err, err)
				op.Error(err)

				if !yield(arriving) {
					return
				}

				continue
			}

			if hasSnapshot {
				if m.Provenance == nil {
					m.Provenance = make(map[string]string)
				}

				m.Provenance["largest_move_symbol"] = snapshot.ExtremeKey

				positive := float64(snapshot.PositiveCount)
				negative := float64(snapshot.NegativeCount)
				zero := float64(snapshot.ZeroCount)
				valid := float64(snapshot.Count)
				peerCount := float64(snapshot.Count - 1)

				m.Metrics["advance_count"] = m.Metrics["advance_count"].Write(positive)
				m.Metrics["decline_count"] = m.Metrics["decline_count"].Write(negative)
				m.Metrics["unchanged_count"] = m.Metrics["unchanged_count"].Write(zero)
				m.Metrics["valid_member_count"] = m.Metrics["valid_member_count"].Write(valid)
				m.Metrics["cohort_member_count"] = m.Metrics["cohort_member_count"].Write(float64(snapshot.TotalMembers))
				m.Metrics["excluded_member_count"] = m.Metrics["excluded_member_count"].Write(float64(snapshot.TotalMembers - snapshot.Count))

				if valid != 0 {
					m.Metrics["advance_fraction"] = m.Metrics["advance_fraction"].Write(positive / valid)
					m.Metrics["decline_fraction"] = m.Metrics["decline_fraction"].Write(negative / valid)
					m.Metrics["unchanged_fraction"] = m.Metrics["unchanged_fraction"].Write(zero / valid)
					m.Metrics["directional_participation"] = m.Metrics["directional_participation"].Write((positive + negative) / valid)
				}

				if positive+negative != 0 {
					m.Metrics["directional_agreement"] = m.Metrics["directional_agreement"].Write(math.Max(positive, negative) / (positive + negative))
					m.Metrics["directional_consensus"] = m.Metrics["directional_consensus"].Write(math.Abs(positive-negative) / (positive + negative))
				}

				m.Metrics["same_direction_peer_count"] = m.Metrics["same_direction_peer_count"].Write(float64(snapshot.SameDirectionCount))
				m.Metrics["opposite_direction_peer_count"] = m.Metrics["opposite_direction_peer_count"].Write(float64(snapshot.OppositeDirectionCount))
				m.Metrics["zero_return_peer_count"] = m.Metrics["zero_return_peer_count"].Write(float64(snapshot.ZeroDirectionCount))

				if peerCount != 0 {
					m.Metrics["same_direction_peer_fraction"] = m.Metrics["same_direction_peer_fraction"].Write(float64(snapshot.SameDirectionCount) / peerCount)
					m.Metrics["opposite_direction_peer_fraction"] = m.Metrics["opposite_direction_peer_fraction"].Write(float64(snapshot.OppositeDirectionCount) / peerCount)
					m.Metrics["zero_return_peer_fraction"] = m.Metrics["zero_return_peer_fraction"].Write(float64(snapshot.ZeroDirectionCount) / peerCount)
				}

				breadth := snapshot.Aggregates["signed_fraction"]

				m.Metrics["breadth"] = m.Metrics["breadth"].Write(breadth.Value)

				if breadth.Ready {
					m.Metrics["breadth_baseline"] = m.Metrics["breadth_baseline"].Write(breadth.Baseline)
					m.Metrics["breadth_divergence"] = m.Metrics["breadth_divergence"].Write(breadth.Divergence)
					m.Metrics["breadth_zscore"] = m.Metrics["breadth_zscore"].Write(breadth.ZScore)
					m.Metrics["breadth_velocity"] = m.Metrics["breadth_velocity"].Write(breadth.Velocity)

					if breadth.NoiseVariance > 0 {
						m.Metadata[data.MetadataDivergence] = breadth.Divergence
						m.Metadata[data.MetadataNoiseVariance] = breadth.NoiseVariance
					}
				}

				signedMedian := snapshot.Aggregates["signed_median"]

				m.Metrics["median_return"] = m.Metrics["median_return"].Write(signedMedian.Value)

				if signedMedian.Ready {
					m.Metrics["median_return_baseline"] = m.Metrics["median_return_baseline"].Write(signedMedian.Baseline)
					m.Metrics["median_return_divergence"] = m.Metrics["median_return_divergence"].Write(signedMedian.Divergence)
					m.Metrics["median_return_zscore"] = m.Metrics["median_return_zscore"].Write(signedMedian.ZScore)
					m.Metrics["median_return_velocity"] = m.Metrics["median_return_velocity"].Write(signedMedian.Velocity)
				}

				medianAbsolute := snapshot.Aggregates["median_absolute"]

				m.Metrics["median_absolute_return"] = m.Metrics["median_absolute_return"].Write(medianAbsolute.Value)

				if medianAbsolute.Ready {
					m.Metrics["median_absolute_return_baseline"] = m.Metrics["median_absolute_return_baseline"].Write(medianAbsolute.Baseline)
					m.Metrics["median_absolute_return_zscore"] = m.Metrics["median_absolute_return_zscore"].Write(medianAbsolute.ZScore)
					m.Metrics["median_absolute_return_velocity"] = m.Metrics["median_absolute_return_velocity"].Write(medianAbsolute.Velocity)

					if medianAbsolute.Baseline != 0 {
						m.Metrics["median_absolute_return_ratio"] = m.Metrics["median_absolute_return_ratio"].Write(medianAbsolute.Value / medianAbsolute.Baseline)
					}
				}

				m.Metrics["mean_absolute_return"] = m.Metrics["mean_absolute_return"].Write(snapshot.Aggregates["mean_absolute"].Value)
				m.Metrics["rms_return"] = m.Metrics["rms_return"].Write(snapshot.Aggregates["rms"].Value)

				iqr := snapshot.Aggregates["iqr"]

				m.Metrics["return_interquartile_range"] = m.Metrics["return_interquartile_range"].Write(iqr.Value)

				if iqr.Ready {
					m.Metrics["return_dispersion_baseline"] = m.Metrics["return_dispersion_baseline"].Write(iqr.Baseline)
					m.Metrics["return_dispersion_zscore"] = m.Metrics["return_dispersion_zscore"].Write(iqr.ZScore)
					m.Metrics["return_dispersion_velocity"] = m.Metrics["return_dispersion_velocity"].Write(iqr.Velocity)

					if iqr.Baseline != 0 {
						m.Metrics["return_dispersion_ratio"] = m.Metrics["return_dispersion_ratio"].Write(iqr.Value / iqr.Baseline)
					}
				}

				m.Metrics["return_mad"] = m.Metrics["return_mad"].Write(snapshot.Mad)
				m.Metrics["magnitude_mad"] = m.Metrics["magnitude_mad"].Write(snapshot.MagnitudeMad)

				m.Metrics["largest_absolute_return"] = m.Metrics["largest_absolute_return"].Write(snapshot.ExtremeMagnitude)
				m.Metrics["largest_move_tie_count"] = m.Metrics["largest_move_tie_count"].Write(float64(snapshot.ExtremeTieCount))

				if snapshot.ExtremeTieCount == 0 {
					m.Metrics["largest_signed_return"] = m.Metrics["largest_signed_return"].Write(snapshot.ExtremeSigned)
				}

				m.Metrics["largest_move_excess"] = m.Metrics["largest_move_excess"].Write(snapshot.ExtremeMagnitude - snapshot.PeerMedianAbsolute)

				if snapshot.PeerMad != 0 {
					m.Metrics["largest_move_mad_excess"] = m.Metrics["largest_move_mad_excess"].Write(
						(snapshot.ExtremeMagnitude - snapshot.PeerMedianAbsolute) / snapshot.PeerMad,
					)
				}

				extremeRatio := snapshot.Aggregates["extreme_ratio"]

				m.Metrics["largest_move_ratio"] = m.Metrics["largest_move_ratio"].Write(extremeRatio.Value)

				if extremeRatio.Ready {
					m.Metrics["largest_move_ratio_baseline"] = m.Metrics["largest_move_ratio_baseline"].Write(extremeRatio.Baseline)
					m.Metrics["largest_move_ratio_zscore"] = m.Metrics["largest_move_ratio_zscore"].Write(extremeRatio.ZScore)
				}

				extremeShare := snapshot.Aggregates["extreme_share"]

				m.Metrics["largest_move_share"] = m.Metrics["largest_move_share"].Write(extremeShare.Value)

				if extremeShare.Ready {
					m.Metrics["largest_move_share_baseline"] = m.Metrics["largest_move_share_baseline"].Write(extremeShare.Baseline)
					m.Metrics["largest_move_share_zscore"] = m.Metrics["largest_move_share_zscore"].Write(extremeShare.ZScore)
				}

				m.Metrics["peer_median_absolute_return"] = m.Metrics["peer_median_absolute_return"].Write(snapshot.PeerMedianAbsolute)
				m.Metrics["peer_magnitude_mad"] = m.Metrics["peer_magnitude_mad"].Write(snapshot.PeerMad)

				m.Metrics["median_asof_age_seconds"] = m.Metrics["median_asof_age_seconds"].Write(snapshot.MedianAge)
				m.Metrics["max_asof_age_seconds"] = m.Metrics["max_asof_age_seconds"].Write(snapshot.MaxAge)
				m.Metrics["median_from_age_seconds"] = m.Metrics["median_from_age_seconds"].Write(snapshot.MedianFromAge)
				m.Metrics["cohort_horizon_seconds"] = m.Metrics["cohort_horizon_seconds"].Write(snapshot.MaxAge)
				m.Metrics["asof_age_seconds"] = m.Metrics["asof_age_seconds"].Write(snapshot.FocalAge)
				m.Metrics["from_age_seconds"] = m.Metrics["from_age_seconds"].Write(snapshot.FocalFromAge)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Fold) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
