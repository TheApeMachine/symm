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
metrics, metadata, and provenance. Peers are encoded one level deep only —
recursive MeasurementToWire on peer.Peers exploded FlatBuffer alloc when the
disruptor Contribute forest was still attached.
*/
func MeasurementToWire(
	measurement *data.Measurement,
	alloc data.Allocator,
	includePeers ...bool,
) *wire.MeasurementT {
	withPeers := false
	if len(includePeers) > 0 {
		withPeers = includePeers[0]
	}
	return measurementToWire(measurement, alloc, withPeers)
}

func measurementToWire(
	measurement *data.Measurement,
	alloc data.Allocator,
	includePeers bool,
) *wire.MeasurementT {
	if measurement == nil || measurement.GetSource() == "cross-section" {
		return nil
	}

	metrics := data.MakeSlice[*wire.MetricT](alloc, 0, len(measurement.Metrics))
	measurement.RangeMetrics(func(key string, metric data.Metric) bool {
		wireMetric := data.New[wire.MetricT](alloc)
		name := metric.Label
		if name == "" {
			name = key
		}
		wireMetric.Name = name
		wireMetric.Raw = metric.Raw
		wireMetric.Unit = string(metric.Unit)
		wireMetric.Region = metric.Region
		wireMetric.X = metric.X
		wireMetric.Y = metric.Y

		if metric.Normalized != nil {
			wireMetric.Normalized = *metric.Normalized
			wireMetric.HasNormalized = true
		} else if metric.Standardized != nil {
			wireMetric.Normalized = *metric.Standardized
			wireMetric.HasNormalized = true
		}

		metrics = data.AppendA(metrics, wireMetric, alloc)
		return true
	})

	provenance := data.MakeSlice[*wire.NamedStringT](alloc, 0, len(measurement.Provenance)+len(measurement.Metadata))
	seenProvenance := make(map[string]struct{}, len(measurement.Provenance))
	measurement.RangeProvenance(func(key, val string) bool {
		ns := data.New[wire.NamedStringT](alloc)
		ns.Name = key
		ns.Value = val
		provenance = data.AppendA(provenance, ns, alloc)
		seenProvenance[key] = struct{}{}
		return true
	})

	metadata := data.MakeSlice[*wire.NamedNumberT](alloc, 0, len(measurement.Metadata))
	measurement.RangeMetadata(func(key, val string) bool {
		floatVal, err := strconv.ParseFloat(val, 64)
		if err == nil {
			nn := data.New[wire.NamedNumberT](alloc)
			nn.Name = key
			nn.Value = floatVal
			metadata = data.AppendA(metadata, nn, alloc)
			return true
		}

		if _, exists := seenProvenance[key]; !exists {
			ns := data.New[wire.NamedStringT](alloc)
			ns.Name = key
			ns.Value = val
			provenance = data.AppendA(provenance, ns, alloc)
		}
		return true
	})

	var peers []*wire.MeasurementT
	if includePeers && len(measurement.Peers) > 0 {
		peers = data.MakeSlice[*wire.MeasurementT](alloc, 0, len(measurement.Peers))
		for _, peer := range measurement.Peers {
			if peer == nil || peer.GetSource() == "cross-section" {
				continue
			}
			// Depth 1 only: peer maps, never peer.Peers.
			if wirePeer := measurementToWire(peer, alloc, false); wirePeer != nil {
				peers = data.AppendA(peers, wirePeer, alloc)
			}
		}
	}

	row := data.New[wire.MeasurementT](alloc)
	row.Source = measurement.GetSource()
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
	measurements []*data.Measurement,
) ([]byte, error) {
	alloc := data.NewAllocator()
	defer data.Free(alloc)

	rows := data.MakeSlice[*wire.MeasurementT](alloc, 0, len(measurements))

	for _, measurement := range measurements {
		if wireMeasurement := MeasurementToWire(measurement, alloc, true); wireMeasurement != nil {
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

func fnv1a32(key string) uint32 {
	var hash uint32 = 2166136261
	for index := 0; index < len(key); index++ {
		hash ^= uint32(key[index])
		hash *= 16777619
	}
	return hash
}
