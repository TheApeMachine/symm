package data

import (
	"context"
	"sync"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	icetable "github.com/apache/iceberg-go/table"
	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/theapemachine/errnie"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
Encode writes the Measurement's complete state into the supplied Arrow RecordBuilder.
*/
func (measurement *Measurement) Encode(recordBuilder *array.RecordBuilder, epoch ...int64) error {
	if measurement == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[data.encoder] measurement is nil",
			nil,
		))
	}

	recEpoch := measurement.Epoch

	if len(epoch) > 0 && epoch[0] > 0 && recEpoch <= 0 {
		recEpoch = epoch[0]
	}

	if recEpoch <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[data.encoder] measurement epoch is non-positive",
			nil,
		), "label", measurement.Label, "source", measurement.Source)
	}

	idBuilder := recordBuilder.Field(0).(*array.Int64Builder)
	epochBuilder := recordBuilder.Field(1).(*array.Int64Builder)
	labelBuilder := recordBuilder.Field(2).(*array.StringBuilder)
	sourceBuilder := recordBuilder.Field(3).(*array.StringBuilder)
	seqIdxBuilder := recordBuilder.Field(4).(*array.Int64Builder)
	tickBuilder := recordBuilder.Field(5).(*array.Int64Builder)
	timestampBuilder := recordBuilder.Field(6).(*array.Int64Builder)
	atBuilder := recordBuilder.Field(7).(*array.TimestampBuilder)
	fromBuilder := recordBuilder.Field(8).(*array.TimestampBuilder)
	maturityBuilder := recordBuilder.Field(9).(*array.Float64Builder)
	samplesBuilder := recordBuilder.Field(10).(*array.Int64Builder)
	energyBuilder := recordBuilder.Field(11).(*array.Float64Builder)
	predictionBuilder := recordBuilder.Field(12).(*array.Float64Builder)
	metricsBuilder := recordBuilder.Field(13).(*array.MapBuilder)
	metadataBuilder := recordBuilder.Field(14).(*array.MapBuilder)

	idBuilder.Append(int64(measurement.ID))
	epochBuilder.Append(recEpoch)
	labelBuilder.Append(measurement.Label)
	sourceBuilder.Append(measurement.Source)
	seqIdxBuilder.Append(measurement.SeqIdx)
	tickBuilder.Append(measurement.Tick)
	timestampBuilder.Append(measurement.Timestamp)
	atBuilder.Append(arrow.Timestamp(measurement.At.UTC().UnixMicro()))
	fromBuilder.Append(arrow.Timestamp(measurement.From.UTC().UnixMicro()))
	maturityBuilder.Append(measurement.maturity)
	samplesBuilder.Append(measurement.samples)
	energyBuilder.Append(measurement.energy)
	predictionBuilder.Append(measurement.prediction)

	metricsKey := metricsBuilder.KeyBuilder().(*array.StringBuilder)
	itemBuilder := metricsBuilder.ItemBuilder().(*array.StructBuilder)
	rawBuilder := itemBuilder.FieldBuilder(0).(*array.Float64Builder)
	snmBuilder := itemBuilder.FieldBuilder(1).(*array.Float64Builder)
	exactBuilder := itemBuilder.FieldBuilder(2).(*array.StringBuilder)
	unitBuilder := itemBuilder.FieldBuilder(3).(*array.StringBuilder)
	timescaleBuilder := itemBuilder.FieldBuilder(4).(*array.StringBuilder)

	metricsBuilder.Append(true)

	for _, entry := range measurement.metrics {
		if entry == nil || entry.Metric == nil {
			continue
		}

		metricsKey.Append(entry.Key)
		itemBuilder.Append(true)
		rawBuilder.Append(entry.Metric.raw)
		snmBuilder.Append(entry.Metric.snm)

		if entry.Metric.exact != nil {
			exactBuilder.Append(entry.Metric.exact.String())
		}

		if entry.Metric.exact == nil {
			exactBuilder.AppendNull()
		}

		unitBuilder.Append(string(entry.Metric.Unit()))
		timescaleBuilder.Append(string(entry.Metric.Timescale()))
	}

	metadataKey := metadataBuilder.KeyBuilder().(*array.StringBuilder)
	metadataVal := metadataBuilder.ItemBuilder().(*array.StringBuilder)
	metadataBuilder.Append(true)

	for _, entry := range measurement.metadata {
		if entry != nil && entry.Key != "" {
			metadataKey.Append(entry.Key)
			metadataVal.Append(entry.Value)
		}
	}

	return nil
}

func (measurement *Measurement) ToParquet(ctx context.Context, tbl *icetable.Table) error {
	if measurement == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[data.encoder] measurement is nil",
			nil,
		))
	}

	if ctx == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[data.encoder] context is nil",
			nil,
		))
	}

	if tbl == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[data.encoder] table is nil",
			nil,
		))
	}

	converted, err := icetable.SchemaToArrowSchema(tbl.Schema(), nil, true, false)
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.encoder] failed to convert table schema to arrow schema",
			err,
		))
	}

	b := array.NewRecordBuilder(memory.DefaultAllocator, converted)
	defer b.Release()

	if err := measurement.Encode(b, measurement.Epoch); err != nil {
		return err
	}

	rec := b.NewRecordBatch()
	defer rec.Release()

	arrowTbl := array.NewTableFromRecords(converted, []arrow.RecordBatch{rec})
	defer arrowTbl.Release()

	if _, err := tbl.AppendTable(ctx, arrowTbl, 1024 /* batchSize */, nil); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[data.encoder] failed to append table",
			err,
		))
	}

	return nil
}

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

