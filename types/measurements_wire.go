package types

import (
	"strconv"
	"sync"

	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/theapemachine/symm/nomagique/data"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

var measurementsBuilderPool = sync.Pool{
	New: func() any {
		return flatbuffers.NewBuilder(131072)
	},
}

/*
EncodeMeasurementsFrameWith serializes a batch of measurements and passes the
borrowed FlatBuffer bytes directly to fn, returning the builder to the pool once
fn returns. This avoids defensive heap cloning when writing directly to sockets.
*/
func EncodeMeasurementsFrameWith(
	measurements []*data.Measurement[float64],
	fn func([]byte) error,
) error {
	rows := make([]*wire.MeasurementT, 0, len(measurements))

	for _, m := range measurements {
		if m == nil {
			continue
		}

		metrics := make([]*wire.MetricT, 0, len(m.Metrics))
		for _, metric := range m.Metrics {
			met := &wire.MetricT{
				Name: metric.Label,
				Raw:  metric.Raw,
				Unit: string(metric.Unit),
			}

			if metric.Normalized != nil {
				met.Normalized = *metric.Normalized
				met.HasNormalized = true
			}

			metrics = append(metrics, met)
		}

		metadata := make([]*wire.NamedNumberT, 0, len(m.Metadata))
		for k, v := range m.Metadata {
			if fval, err := strconv.ParseFloat(v, 64); err == nil {
				metadata = append(metadata, &wire.NamedNumberT{
					Name:  k,
					Value: fval,
				})
			}
		}

		rows = append(rows, &wire.MeasurementT{
			Source:       m.Source,
			Symbol:       m.Label,
			Tick:         m.SeqIdx,
			At:           m.At.UnixNano(),
			ObservedFrom: m.From.UnixNano(),
			Maturity:     m.Maturity,
			Snr:          m.SNR,
			SnrDefined:   m.SNRDefined,
			Metrics:      metrics,
			Metadata:     metadata,
		})
	}

	if len(rows) == 0 {
		return nil
	}

	frame := &wire.MeasurementsFrameT{
		Rows: rows,
	}

	builder := measurementsBuilderPool.Get().(*flatbuffers.Builder)
	builder.Reset()
	defer measurementsBuilderPool.Put(builder)

	offset := frame.Pack(builder)
	builder.Finish(offset)

	return fn(builder.FinishedBytes())
}
