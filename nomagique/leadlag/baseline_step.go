package leadlag

import (
	"iter"
	"strconv"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
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

func NewBaselineStep(window func() core.Primitive) core.Primitive {
	return &BaselineStep{
		PrimitiveError: core.NewPrimitiveError(),
		lag:            adaptive.NewBaseline(window()),
		gain:           adaptive.NewBaseline(window()),
		corr:           adaptive.NewBaseline(window()),
	}
}

func (op *BaselineStep) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			lagVal := m.GetMetric("best_lag_seconds").Raw
			gainVal := m.GetMetric("absolute_correlation_gain").Raw
			corrVal := m.GetMetric("best_lag_correlation").Raw

			lagReading := op.drive(op.lag, lagVal)
			gainReading := op.drive(op.gain, gainVal)
			corrReading := op.drive(op.corr, corrVal)

			m.WriteMetric("lag_baseline_seconds", lagReading.Baseline)
			m.WriteMetric("lag_divergence_seconds", lagReading.Residual)
			m.WriteMetric("lag_zscore", lagReading.ZScore)

			if lagReading.VarianceDefined {
				m.WriteMetric("lag_noise_scale_seconds", lagReading.Dispersion)
			}

			m.WriteMetric("correlation_gain_baseline", gainReading.Baseline)
			m.WriteMetric("correlation_gain_zscore", gainReading.ZScore)

			m.WriteMetric("best_lag_correlation_baseline", corrReading.Baseline)
			m.WriteMetric("best_lag_correlation_zscore", corrReading.ZScore)

			m.EnsureMetadata()

			m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(corrReading.Count, 'f', -1, 64))

			if corrReading.HasPrior {
				m.SetMetadata(data.MetadataDivergence, strconv.FormatFloat(corrReading.Residual, 'f', -1, 64))
			}

			if corrReading.VarianceDefined {
				m.SetMetadata(data.MetadataNoiseVariance, strconv.FormatFloat(corrReading.Variance, 'f', -1, 64))
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *BaselineStep) drive(baseline core.Primitive, value float64) adaptive.BaselineReading {
	var reading adaptive.BaselineReading

	for out := range baseline.Next(transport.NewOne(unsafe.Pointer(&value)).Next(nil)) {
		reading = *(*adaptive.BaselineReading)(out)
	}

	if err := baseline.Error(); err != nil {
		op.Error(err)
	}

	return reading
}
