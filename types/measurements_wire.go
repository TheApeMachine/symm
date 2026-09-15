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
/*
MeasurementToWire converts a data.Measurement to wire.MeasurementT including
metrics, metadata, and provenance. Peers stay off the dashboard frame: they are
live register snapshots, and encoding the tree made half-megabyte websocket
messages that stalled the UI.
*/
func MeasurementToWire(measurement *data.Measurement[float64]) *wire.MeasurementT {
	if measurement == nil {
		return nil
	}

	metrics := make([]*wire.MetricT, 0, len(measurement.Metrics))
	for _, metric := range measurement.Metrics {
		wireMetric := &wire.MetricT{
			Name: metric.Label,
			Raw:  metric.Raw,
			Unit: string(metric.Unit),
		}

		if metric.Normalized != nil {
			wireMetric.Normalized = *metric.Normalized
			wireMetric.HasNormalized = true
		}

		metrics = append(metrics, wireMetric)
	}

	metadata := make([]*wire.NamedNumberT, 0, len(measurement.Metadata))
	for key, val := range measurement.Metadata {
		if floatVal, err := strconv.ParseFloat(val, 64); err == nil {
			metadata = append(metadata, &wire.NamedNumberT{
				Name:  key,
				Value: floatVal,
			})
		}
	}

	provenance := make([]*wire.NamedStringT, 0, len(measurement.Provenance))
	for key, val := range measurement.Provenance {
		provenance = append(provenance, &wire.NamedStringT{
			Name:  key,
			Value: val,
		})
	}

	return &wire.MeasurementT{
		Source:       measurement.Source,
		Symbol:       measurement.Label,
		Tick:         measurement.SeqIdx,
		At:           measurement.At.UnixNano(),
		ObservedFrom: measurement.From.UnixNano(),
		Maturity:     measurement.Maturity,
		Snr:          measurement.SNR,
		SnrDefined:   measurement.SNRDefined,
		Metrics:      metrics,
		Metadata:     metadata,
		Provenance:   provenance,
	}
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

	for _, measurement := range measurements {
		if wireMeasurement := MeasurementToWire(measurement); wireMeasurement != nil {
			rows = append(rows, wireMeasurement)
		}
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
