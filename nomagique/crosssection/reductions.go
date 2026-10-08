package crosssection

import (
	"iter"
	"math"
	"strconv"
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
			m := *(**data.Measurement)(arriving)

			if m.Error() != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			positive, negative, zero := 0.0, 0.0, 0.0
			var extremeKey string
			var maxAbsChange float64

			for _, peer := range m.Peers() {
				change := peer.Value("change")
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

			m.Put("valid_member_count", valid)
			m.Put("positive_count", positive)
			m.Put("negative_count", negative)
			m.Put("zero_count", zero)

			if valid > 0 {
				m.Put("signed_fraction", (positive-negative)/valid)

				if extremeKey != "" {
					m.SetMeta("extreme_key", extremeKey)
				}
			}

			m.SetMeta(data.MetadataSupport, strconv.FormatFloat(valid, 'f', -1, 64))

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
			m := *(**data.Measurement)(arriving)

			if m.Error() != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			valid := m.Value("valid_member_count")

			if valid > 0 {
				fraction := m.Value("signed_fraction")
				reading := drive[float64, adaptive.BaselineReading](op.baseline, &fraction)

				if reading.HasPrior {
					m.Put("signed_fraction_baseline", reading.Baseline)
					m.Put("signed_fraction_divergence", reading.Residual)
					m.Put("signed_fraction_zscore", reading.ZScore)

					m.SetMeta(data.MetadataDivergence, strconv.FormatFloat(reading.Residual, 'f', -1, 64))

					if reading.VarianceDefined {
						m.SetMeta(data.MetadataNoiseVariance, strconv.FormatFloat(reading.Variance, 'f', -1, 64))
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
			m := *(**data.Measurement)(arriving)

			if m.Error() != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			if len(m.Peers()) == 0 {
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
				m.SetError(err)
				op.Error(err)

				if !yield(arriving) {
					return
				}

				continue
			}

			m.Put("signed_median", median)

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
func peerValues(m *data.Measurement) []float64 {
	changes := make([]float64, 0, len(m.Peers()))

	for _, peer := range m.Peers() {
		changes = append(changes, peer.Value("change"))
	}

	return changes
}
