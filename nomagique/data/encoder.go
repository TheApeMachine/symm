package data

import (
	"sync"

	flatbuffers "github.com/google/flatbuffers/go"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

var measurementsBuilderPool = sync.Pool{
	New: func() any {
		return flatbuffers.NewBuilder(131072)
	},
}

/*
ToWire converts a Measurement to wire.MeasurementT including
metrics, metadata, and provenance. Peers are encoded one level deep only.
*/
func (measurement *Measurement) ToWire(
	alloc Allocator,
	includePeers ...bool,
) *wire.MeasurementT {
	if measurement == nil || measurement.Source == "cross-section" || measurement.Source == "runtime:join" {
		return nil
	}

	withPeers := false
	if len(includePeers) > 0 {
		withPeers = includePeers[0]
	}

	metrics := MakeSlice[*wire.MetricT](alloc, 0, len(measurement.metrics))

	for _, entry := range measurement.metrics {
		if entry == nil || entry.Metric == nil || entry.Err != nil {
			continue
		}

		wireMetric := New[wire.MetricT](alloc)
		name := entry.Metric.label

		if name == "" {
			name = entry.Key
		}

		wireMetric.Name = name
		wireMetric.Raw = entry.Metric.raw
		wireMetric.Snm = entry.Metric.snm

		metrics = AppendA(metrics, wireMetric, alloc)
	}

	provenance := MakeSlice[*wire.NamedStringT](alloc, 0, 8)
	metadata := MakeSlice[*wire.NamedNumberT](alloc, 0, 8)

	if measurement.err != nil {
		namedString := New[wire.NamedStringT](alloc)
		namedString.Name = "symm:error"
		namedString.Value = measurement.err.Error()
		provenance = AppendA(provenance, namedString, alloc)
	}

	for _, entry := range measurement.metrics {
		if entry == nil || entry.Metric == nil || entry.Err != nil || entry.Metric.exact == nil {
			continue
		}

		namedString := New[wire.NamedStringT](alloc)
		namedString.Name = "symm:exact:" + entry.Key
		namedString.Value = entry.Metric.exact.String()
		provenance = AppendA(provenance, namedString, alloc)
	}

	for _, entry := range measurement.metadata {
		if entry == nil || entry.Key == "" {
			continue
		}
		namedString := New[wire.NamedStringT](alloc)
		namedString.Name = entry.Key
		namedString.Value = entry.Value
		provenance = AppendA(provenance, namedString, alloc)
	}

	var peers []*wire.MeasurementT

	if withPeers && len(measurement.peers) > 0 {
		peers = MakeSlice[*wire.MeasurementT](alloc, 0, len(measurement.peers))

		for _, peer := range measurement.peers {
			if peer == nil || peer.Source == "cross-section" || peer.Source == "runtime:join" {
				continue
			}

			// Depth 1 only: peer maps, never peer.Peers.
			if wirePeer := peer.ToWire(alloc, false); wirePeer != nil {
				peers = AppendA(peers, wirePeer, alloc)
			}
		}
	}

	row := New[wire.MeasurementT](alloc)
	row.Source = measurement.Source
	row.Symbol = measurement.Label
	row.Tick = measurement.SeqIdx
	row.At = measurement.At.UnixNano()
	row.ObservedFrom = measurement.From.UnixNano()
	row.Maturity = measurement.maturity
	row.Metrics = metrics
	row.Metadata = metadata
	row.Provenance = provenance
	row.Peers = peers

	return row
}

/*
EncodeMeasurements serializes a batch of measurements to FlatBuffers bytes.
*/
func EncodeMeasurements(
	measurements []*Measurement,
) ([]byte, error) {
	alloc := NewAllocator()
	defer Free(alloc)

	rows := MakeSlice[*wire.MeasurementT](alloc, 0, len(measurements))

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}
		if wireMeasurement := measurement.ToWire(alloc, true); wireMeasurement != nil {
			rows = AppendA(rows, wireMeasurement, alloc)
		}
	}

	if len(rows) == 0 {
		return nil, nil
	}

	frame := New[wire.MeasurementsFrameT](alloc)
	frame.Rows = rows

	builder := measurementsBuilderPool.Get().(*flatbuffers.Builder)
	builder.Reset()
	defer measurementsBuilderPool.Put(builder)

	offset := frame.Pack(builder)
	builder.Finish(offset)

	return append([]byte{}, builder.FinishedBytes()...), nil
}

/*
PackMeasurementsFrame encodes pre-built wire.MeasurementT rows directly into
a FlatBuffer MeasurementsFrame payload.
*/
func PackMeasurementsFrame(rows []*wire.MeasurementT) []byte {
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

	return append([]byte{}, builder.FinishedBytes()...)
}
