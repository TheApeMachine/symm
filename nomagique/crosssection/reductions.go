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
	err      error
	baseline core.Primitive
}

func NewChangeCounts() core.Primitive {
	return &ChangeCounts{
		baseline: adaptive.NewBaseline(adaptive.NewWindow()),
	}
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
				change := peer.GetMetric("change").Raw
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

			reading := drive[float64, adaptive.BaselineReading](op.baseline, &valid)

			validCenter := valid
			validScale := 1.0
			if reading.HasPrior {
				validCenter = reading.Baseline
				validScale = math.Max(reading.Dispersion, 1.0)
			}

			m.SetMetric("valid_member_count", data.NewMetric[float64](
				"valid_member_count",
				data.UnitCount,
				data.TimescaleInstantaneous,
				validCenter,
				validScale,
			).Write(valid))

			// Binomial null hypothesis: unbiased market has p = 0.5 up vs down.
			// Expected mean = valid * 0.5, Standard deviation = sqrt(valid * 0.25) = 0.5 * sqrt(valid)
			binomialCenter := valid * 0.5
			binomialScale := math.Max(0.5*math.Sqrt(valid), 0.5)

			m.SetMetric("positive_count", data.NewMetric[float64](
				"positive_count",
				data.UnitCount,
				data.TimescaleInstantaneous,
				binomialCenter,
				binomialScale,
			).Write(positive))
			m.SetMetric("negative_count", data.NewMetric[float64](
				"negative_count",
				data.UnitCount,
				data.TimescaleInstantaneous,
				binomialCenter,
				binomialScale,
			).Write(negative))
			m.SetMetric("zero_count", data.NewMetric[float64](
				"zero_count",
				data.UnitCount,
				data.TimescaleInstantaneous,
				0,
				math.Max(math.Sqrt(valid), 1.0),
			).Write(zero))

			if valid > 0 {
				m.SetMetric("signed_fraction", data.NewMetric[float64](
					"signed_fraction",
					data.UnitRatio,
					data.TimescaleInstantaneous,
					0.0,
					1.0,
				).Write((positive - negative) / valid))

				if extremeKey != "" {
					m.SetProvenance("extreme_key", extremeKey)
				}
			}

			m.EnsureMetadata()

			m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(valid, 'f', -1, 64))

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

			valid := m.GetMetric("valid_member_count").Raw

			if valid > 0 {
				fraction := m.GetMetric("signed_fraction").Raw
				reading := drive[float64, adaptive.BaselineReading](op.baseline, &fraction)

				if reading.HasPrior {
					dispersion := math.Max(reading.Dispersion, 1e-6)
					m.SetMetric("signed_fraction_baseline", data.NewMetric[float64](
						"signed_fraction_baseline",
						data.UnitRatio,
						data.TimescaleRollingWindow,
						0.0,
						1.0,
					).Write(reading.Baseline))
					m.SetMetric("signed_fraction_divergence", data.NewMetric[float64](
						"signed_fraction_divergence",
						data.UnitRatio,
						data.TimescaleInstantaneous,
						0.0,
						dispersion,
					).Write(reading.Residual))
					m.SetMetric("signed_fraction_zscore", data.NewMetric[float64](
						"signed_fraction_zscore",
						data.UnitZScore,
						data.TimescaleRollingWindow,
						0.0,
						1.0,
					).Write(reading.ZScore))
					m.EnsureMetadata()

					m.SetMetadata(data.MetadataDivergence, strconv.FormatFloat(reading.Residual, 'f', -1, 64))

					if reading.VarianceDefined {
						m.SetMetadata(data.MetadataNoiseVariance, strconv.FormatFloat(reading.Variance, 'f', -1, 64))
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

			m.SetMetric("signed_median", data.NewMetric[float64](
				"signed_median",
				data.UnitRatio,
				data.TimescaleInstantaneous,
				0.0,
				math.Max(math.Abs(median), 1e-6),
			).Write(median))

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
		changes = append(changes, peer.GetMetric("change").Raw)
	}

	return changes
}
