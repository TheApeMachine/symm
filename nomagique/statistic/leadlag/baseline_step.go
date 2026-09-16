package leadlag

import (
	"iter"
	"strconv"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
)

/*
BaselineStep owns three adaptive baselines — lag, correlation gain, and
best-lag correlation — and stamps each baseline's reading onto the
measurement. Metadata support, divergence, and noise variance are also
stamped.
*/
type BaselineStep struct {
	*core.PrimitiveError
	lag  core.Primitive
	gain core.Primitive
	corr core.Primitive
}

func NewBaselineStep(window func() *adaptive.Window) *BaselineStep {
	return &BaselineStep{
		PrimitiveError: core.NewPrimitiveError(),
		lag:            adaptive.NewBaseline(window()),
		gain:           adaptive.NewBaseline(window()),
		corr:           adaptive.NewBaseline(window()),
	}
}

func (baselineStep *BaselineStep) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			lagVal := m.Metrics["best_lag_seconds"].Raw
			gainVal := m.Metrics["absolute_correlation_gain"].Raw
			corrVal := m.Metrics["best_lag_correlation"].Raw

			lagReading := baselineStep.drive(baselineStep.lag, lagVal)
			gainReading := baselineStep.drive(baselineStep.gain, gainVal)
			corrReading := baselineStep.drive(baselineStep.corr, corrVal)

			m.Metrics["lag_baseline_seconds"] = m.Metrics["lag_baseline_seconds"].Write(lagReading.Baseline)
			m.Metrics["lag_divergence_seconds"] = m.Metrics["lag_divergence_seconds"].Write(lagReading.Residual)
			m.Metrics["lag_zscore"] = m.Metrics["lag_zscore"].Write(lagReading.ZScore)

			if lagReading.VarianceDefined {
				m.Metrics["lag_noise_scale_seconds"] = m.Metrics["lag_noise_scale_seconds"].Write(lagReading.Dispersion)
			}

			m.Metrics["correlation_gain_baseline"] = m.Metrics["correlation_gain_baseline"].Write(gainReading.Baseline)
			m.Metrics["correlation_gain_zscore"] = m.Metrics["correlation_gain_zscore"].Write(gainReading.ZScore)

			m.Metrics["best_lag_correlation_baseline"] = m.Metrics["best_lag_correlation_baseline"].Write(corrReading.Baseline)
			m.Metrics["best_lag_correlation_zscore"] = m.Metrics["best_lag_correlation_zscore"].Write(corrReading.ZScore)

			if m.Metadata == nil {
				m.Metadata = make(map[string]string)
			}

			m.Metadata[data.MetadataSupport] = strconv.FormatFloat(corrReading.Count, 'f', -1, 64)

			if corrReading.HasPrior {
				m.Metadata[data.MetadataDivergence] = strconv.FormatFloat(corrReading.Residual, 'f', -1, 64)
			}

			if corrReading.VarianceDefined {
				m.Metadata[data.MetadataNoiseVariance] = strconv.FormatFloat(corrReading.Variance, 'f', -1, 64)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (baselineStep *BaselineStep) drive(baseline core.Primitive, value float64) adaptive.BaselineReading {
	var reading adaptive.BaselineReading

	for out := range baseline.Next(sequence.NewOne(unsafe.Pointer(&value)).Next(nil)) {
		reading = *(*adaptive.BaselineReading)(out)
	}

	if err := baseline.Error(); err != nil {
		baselineStep.Error(err)
	}

	return reading
}
