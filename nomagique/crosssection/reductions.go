package crosssection

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
ChangeCounts takes the sign census of the member changes and writes the sign
counts, the valid member count, and the signed fraction equation
(positive - negative) / valid.
*/
type ChangeCounts struct {
	err error
}

func NewChangeCounts() core.Primitive {
	return &ChangeCounts{}
}

func (op *ChangeCounts) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			m.Metadata[data.MetadataSupport] = valid

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
	err      error
	baseline core.Primitive
}

func NewChangeBaseline() core.Primitive {
	return &ChangeBaseline{baseline: adaptive.NewBaseline(adaptive.NewWindow())}
}

func (op *ChangeBaseline) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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
				reading := drive[float64, adaptive.BaselineReading](op.baseline, &fraction)

				if reading.HasPrior {
					m.Metrics["signed_fraction_baseline"] = m.Metrics["signed_fraction_baseline"].Write(reading.Baseline)
					m.Metrics["signed_fraction_divergence"] = m.Metrics["signed_fraction_divergence"].Write(reading.Residual)
					m.Metrics["signed_fraction_zscore"] = m.Metrics["signed_fraction_zscore"].Write(reading.ZScore)
					m.Metadata[data.MetadataDivergence] = reading.Residual

					if reading.VarianceDefined {
						m.Metadata[data.MetadataNoiseVariance] = reading.Variance
					}
				}
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *ChangeBaseline) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}

func (op *ChangeCounts) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}

/*
ChangeMedian reduces the member changes to their median.
*/
type ChangeMedian struct {
	err    error
	median core.Primitive
}

func NewChangeMedian() core.Primitive {
	return &ChangeMedian{median: statistic.NewMedian()}
}

func (op *ChangeMedian) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			for out := range op.median.Next(transport.NewValues(peerValues(m)).Next(nil)) {
				median = *(*float64)(out)
			}

			if err := op.median.Error(); err != nil {
				m.Err = err
				op.Error(err)

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

func (op *ChangeMedian) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
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
