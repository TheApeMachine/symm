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
MeasurementToWire converts a data.Measurement to wire.MeasurementT including
metrics, metadata, and provenance.
*/
func MeasurementToWire(measurement *data.Measurement[float64], alloc data.Allocator) *wire.MeasurementT {
	if measurement == nil || measurement.Source == "cross-section" {
		return nil
	}

	metrics := data.MakeSlice[*wire.MetricT](alloc, 0, len(measurement.Metrics))
	for _, metric := range measurement.Metrics {
		wireMetric := data.New[wire.MetricT](alloc)
		wireMetric.Name = metric.Label
		wireMetric.Raw = metric.Raw
		wireMetric.Unit = string(metric.Unit)
		wireMetric.X = metric.X
		wireMetric.Y = metric.Y
		wireMetric.Region = metric.Region

		if metric.Normalized != nil {
			wireMetric.Normalized = *metric.Normalized
			wireMetric.HasNormalized = true
		}

		metrics = data.AppendA(metrics, wireMetric, alloc)
	}

	provenance := data.MakeSlice[*wire.NamedStringT](alloc, 0, len(measurement.Provenance)+len(measurement.Metadata))
	for key, val := range measurement.Provenance {
		ns := data.New[wire.NamedStringT](alloc)
		ns.Name = key
		ns.Value = val
		provenance = data.AppendA(provenance, ns, alloc)
	}

	metadata := data.MakeSlice[*wire.NamedNumberT](alloc, 0, len(measurement.Metadata))
	for key, val := range measurement.Metadata {
		floatVal, err := strconv.ParseFloat(val, 64)
		if err == nil {
			nn := data.New[wire.NamedNumberT](alloc)
			nn.Name = key
			nn.Value = floatVal
			metadata = data.AppendA(metadata, nn, alloc)
			continue
		}

		if _, exists := measurement.GetProvenance(key); !exists {
			ns := data.New[wire.NamedStringT](alloc)
			ns.Name = key
			ns.Value = val
			provenance = data.AppendA(provenance, ns, alloc)
		}
	}

	var peers []*wire.MeasurementT
	if len(measurement.Peers) > 0 {
		peers = data.MakeSlice[*wire.MeasurementT](alloc, 0, len(measurement.Peers))
	}
	
	for _, peer := range measurement.Peers {
		if peer == nil || peer.Source == "cross-section" {
			continue
		}

		if wirePeer := MeasurementToWire(peer, alloc); wirePeer != nil {
			peers = data.AppendA(peers, wirePeer, alloc)
		}
	}

	row := data.New[wire.MeasurementT](alloc)
	row.Source = measurement.Source
	row.Symbol = measurement.Label
	row.Tick = measurement.SeqIdx
	row.At = measurement.At.UnixNano()
	row.ObservedFrom = measurement.From.UnixNano()
	row.Maturity = measurement.Maturity
	row.Snr = measurement.SNR
	row.SnrDefined = measurement.SNRDefined
	row.Metrics = metrics
	row.Metadata = metadata
	row.Provenance = provenance
	row.Peers = peers

	return row
}

/*
EncodeMeasurementsFrameWith serializes a batch of measurements and passes the
borrowed FlatBuffer bytes directly to fn, returning the builder to the pool once
fn returns. This avoids defensive heap cloning when writing directly to sockets.
*/
func EncodeMeasurements(
	measurements []*data.Measurement[float64],
) ([]byte, error) {
	alloc := data.NewAllocator()
	defer data.Free(alloc)

	rows := data.MakeSlice[*wire.MeasurementT](alloc, 0, len(measurements))

	for _, measurement := range measurements {
		if wireMeasurement := MeasurementToWire(measurement, alloc); wireMeasurement != nil {
			rows = data.AppendA(rows, wireMeasurement, alloc)
		}
	}

	if len(rows) == 0 {
		return nil, nil
	}

	frame := data.New[wire.MeasurementsFrameT](alloc)
	frame.Rows = rows

	builder := measurementsBuilderPool.Get().(*flatbuffers.Builder)
	builder.Reset()
	defer measurementsBuilderPool.Put(builder)

	offset := frame.Pack(builder)
	builder.Finish(offset)

	res := append([]byte{}, builder.FinishedBytes()...)

	return res, nil
}
