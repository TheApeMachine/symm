package crosssection

import (
	"iter"
	"math"
	"strconv"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
ChangeCounts takes the sign census of the member changes and writes the sign
counts, the valid member count, and the signed fraction equation
(positive - negative) / valid.
*/
type ChangeCounts struct {
	*core.PrimitiveError
}

func NewChangeCounts() *ChangeCounts {
	return &ChangeCounts{PrimitiveError: core.NewPrimitiveError()}
}

func (changeCounts *ChangeCounts) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			positive, negative, zero := 0.0, 0.0, 0.0
			var extremeKey string
			var maxAbsChange float64

			for _, peer := range m.Peers {
				change := peer.Metrics["change"].Raw
				absChange := math.Abs(change)

				if absChange > maxAbsChange || extremeKey == "" {
					maxAbsChange = absChange
					extremeKey = peer.Label
				}

				switch {
				case change > 0:
					positive++
				case change < 0:
					negative++
				default:
					zero++
				}
			}

			valid := positive + negative + zero

			m.Metrics["valid_member_count"] = m.Metrics["valid_member_count"].Write(valid)
			m.Metrics["positive_count"] = m.Metrics["positive_count"].Write(positive)
			m.Metrics["negative_count"] = m.Metrics["negative_count"].Write(negative)
			m.Metrics["zero_count"] = m.Metrics["zero_count"].Write(zero)

			if valid > 0 {
				m.Metrics["signed_fraction"] = m.Metrics["signed_fraction"].Write((positive - negative) / valid)

				if extremeKey != "" {
					if m.Provenance == nil {
						m.Provenance = make(map[string]string, 1)
					}

					m.Provenance["extreme_key"] = extremeKey
				}
			}

			if m.Metadata == nil {
				m.Metadata = make(map[string]string)
			}

			m.Metadata[data.MetadataSupport] = strconv.FormatFloat(valid, 'f', -1, 64)

			if !yield(arriving) {
				return
			}
		}
	}
}

/*
ChangeBaseline evaluates an adaptive causal baseline over the signed fraction.
*/
type ChangeBaseline struct {
	*core.PrimitiveError

	baseline core.Primitive
}

func NewChangeBaseline() *ChangeBaseline {
	return &ChangeBaseline{PrimitiveError: core.NewPrimitiveError(), baseline: adaptive.NewBaseline(adaptive.NewWindow())}
}

func (changeBaseline *ChangeBaseline) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			valid := m.Metrics["valid_member_count"].Raw

			if valid > 0 {
				fraction := m.Metrics["signed_fraction"].Raw
				reading := drive[float64, adaptive.BaselineReading](changeBaseline.baseline, &fraction)

				if reading.HasPrior {
					m.Metrics["signed_fraction_baseline"] = m.Metrics["signed_fraction_baseline"].Write(reading.Baseline)
					m.Metrics["signed_fraction_divergence"] = m.Metrics["signed_fraction_divergence"].Write(reading.Residual)
					m.Metrics["signed_fraction_zscore"] = m.Metrics["signed_fraction_zscore"].Write(reading.ZScore)
					if m.Metadata == nil {
						m.Metadata = make(map[string]string)
					}

					m.Metadata[data.MetadataDivergence] = strconv.FormatFloat(reading.Residual, 'f', -1, 64)

					if reading.VarianceDefined {
						m.Metadata[data.MetadataNoiseVariance] = strconv.FormatFloat(reading.Variance, 'f', -1, 64)
					}
				}
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

/*
ChangeMedian reduces the member changes to their median.
*/
type ChangeMedian struct {
	*core.PrimitiveError

	median core.Primitive
}

func NewChangeMedian() *ChangeMedian {
	return &ChangeMedian{PrimitiveError: core.NewPrimitiveError(), median: statistic.NewMedian()}
}

func (changeMedian *ChangeMedian) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m.Err != nil {
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

			var median float64

			for out := range changeMedian.median.Next(sequence.NewValues(peerValues(m)).Next(nil)) {
				median = *(*float64)(out)
			}

			if err := changeMedian.median.Error(); err != nil {
				m.Err = err
				changeMedian.Error(err)

				if !yield(arriving) {
					return
				}

				continue
			}

			m.Metrics["signed_median"] = m.Metrics["signed_median"].Write(median)

			if !yield(arriving) {
				return
			}
		}
	}
}

/*
peerValues extracts the peer changes of one cross-section.
*/
func peerValues(m *data.Measurement[float64]) []float64 {
	changes := make([]float64, 0, len(m.Peers))

	for _, peer := range m.Peers {
		changes = append(changes, peer.Metrics["change"].Raw)
	}

	return changes
}
