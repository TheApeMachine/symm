package types

import (
	"strconv"
	"sync"

	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
regions pins each wire metric to the confluence region the grid scores it
in, so the impulse map groups cells exactly as Step's tokens do.
*/
var regions = store.NewGrid()

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
	if measurement == nil || measurement.Source == "cross-section" {
		return nil
	}

	metrics := data.MakeSlice[*wire.MetricT](alloc, 0, 8)

	for entry := range measurement.Read() {
		if entry.Err != nil {
			continue
		}

		wireMetric := data.New[wire.MetricT](alloc)
		name := entry.Metric.Label

		if name == "" {
			name = entry.Key
		}

		wireMetric.Name = name
		wireMetric.Raw = entry.Metric.Raw
		wireMetric.Region = regions.PinRegion(measurement.Source, entry.Key)
		if entry.Metric.Normalized != 0 {
			wireMetric.Normalized = entry.Metric.Normalized
			wireMetric.HasNormalized = true
		}

		if entry.Metric.Normalized == 0 && entry.Metric.Standardized != 0 {
			wireMetric.Normalized = entry.Metric.Standardized
			wireMetric.HasNormalized = true
		}

		metrics = data.AppendA(metrics, wireMetric, alloc)
	}

	provenance := data.MakeSlice[*wire.NamedStringT](alloc, 0, 8)
	metadata := data.MakeSlice[*wire.NamedNumberT](alloc, 0, 8)

	if measurement.Error() != nil {
		namedString := data.New[wire.NamedStringT](alloc)
		namedString.Name = "symm:error"
		namedString.Value = measurement.Error().Error()
		provenance = data.AppendA(provenance, namedString, alloc)
	}

	for entry := range measurement.Read() {
		if entry.Err != nil || entry.Metric.Exact == nil {
			continue
		}

		namedString := data.New[wire.NamedStringT](alloc)
		namedString.Name = "symm:exact:" + entry.Key
		namedString.Value = entry.Metric.Exact.String()
		provenance = data.AppendA(provenance, namedString, alloc)
	}

	for _, entry := range measurement.Metadata() {
		if entry == nil || entry.Key == "" {
			continue
		}

		floatVal, parseErr := strconv.ParseFloat(entry.Value, 64)

		if parseErr == nil {
			namedNumber := data.New[wire.NamedNumberT](alloc)
			namedNumber.Name = entry.Key
			namedNumber.Value = floatVal
			metadata = data.AppendA(metadata, namedNumber, alloc)
			continue
		}

		namedString := data.New[wire.NamedStringT](alloc)
		namedString.Name = entry.Key
		namedString.Value = entry.Value
		provenance = data.AppendA(provenance, namedString, alloc)
	}

	var peers []*wire.MeasurementT
	measurementPeers := measurement.Peers()

	if includePeers && len(measurementPeers) > 0 {
		peers = data.MakeSlice[*wire.MeasurementT](alloc, 0, len(measurementPeers))

		for _, peer := range measurementPeers {
			if peer == nil || peer.Source == "cross-section" {
				continue
			}

			// Depth 1 only: peer maps, never peer.Peers.
			if wirePeer := measurementToWire(peer, alloc, false); wirePeer != nil {
				peers = data.AppendA(peers, wirePeer, alloc)
			}
		}
	}

	row := data.New[wire.MeasurementT](alloc)
	row.Source = measurement.Source
	row.Symbol = measurement.Label
	row.Tick = measurement.SeqIdx
	row.At = measurement.At.UnixNano()
	row.ObservedFrom = measurement.From.UnixNano()
	row.Maturity = measurement.Maturity()
	row.Snr = measurement.Coherence()
	row.SnrDefined = true
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

/*
PackMeasurementsFrame encodes pre-built wire.MeasurementT rows directly into
a FlatBuffer MeasurementsFrame payload without constructing or validating
intermediate domain data.Measurement structs.
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
