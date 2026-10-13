package data

import (
	"context"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	icetable "github.com/apache/iceberg-go/table"
	"github.com/theapemachine/errnie"
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
	coherenceBuilder := recordBuilder.Field(9).(*array.Float64Builder)
	maturityBuilder := recordBuilder.Field(10).(*array.Float64Builder)
	samplesBuilder := recordBuilder.Field(11).(*array.Int64Builder)
	energyBuilder := recordBuilder.Field(12).(*array.Float64Builder)
	predictionBuilder := recordBuilder.Field(13).(*array.Float64Builder)
	metricsBuilder := recordBuilder.Field(14).(*array.MapBuilder)
	metadataBuilder := recordBuilder.Field(15).(*array.MapBuilder)

	idBuilder.Append(int64(measurement.ID))
	epochBuilder.Append(recEpoch)
	labelBuilder.Append(measurement.Label)
	sourceBuilder.Append(measurement.Source)
	seqIdxBuilder.Append(measurement.SeqIdx)
	tickBuilder.Append(measurement.Tick)
	timestampBuilder.Append(measurement.Timestamp)
	atBuilder.Append(arrow.Timestamp(measurement.At.UTC().UnixMicro()))
	fromBuilder.Append(arrow.Timestamp(measurement.From.UTC().UnixMicro()))
	coherenceBuilder.Append(measurement.coherence)
	maturityBuilder.Append(measurement.maturity)
	samplesBuilder.Append(measurement.samples)
	energyBuilder.Append(measurement.energy)
	predictionBuilder.Append(measurement.prediction)

	metricsKey := metricsBuilder.KeyBuilder().(*array.StringBuilder)
	itemBuilder := metricsBuilder.ItemBuilder().(*array.StructBuilder)
	rawBuilder := itemBuilder.FieldBuilder(0).(*array.Float64Builder)
	normBuilder := itemBuilder.FieldBuilder(1).(*array.Float64Builder)
	stdBuilder := itemBuilder.FieldBuilder(2).(*array.Float64Builder)
	exactBuilder := itemBuilder.FieldBuilder(3).(*array.StringBuilder)
	supportBuilder := itemBuilder.FieldBuilder(4).(*array.Float64Builder)
	unitBuilder := itemBuilder.FieldBuilder(5).(*array.StringBuilder)
	timescaleBuilder := itemBuilder.FieldBuilder(6).(*array.StringBuilder)

	metricsBuilder.Append(true)

	for _, entry := range measurement.metrics {
		if entry == nil || entry.Metric == nil {
			continue
		}

		metricsKey.Append(entry.Key)
		itemBuilder.Append(true)
		rawBuilder.Append(entry.Metric.raw)
		normBuilder.Append(entry.Metric.snm)
		stdBuilder.Append(entry.Metric.snm)

		if entry.Metric.exact != nil {
			exactBuilder.Append(entry.Metric.exact.String())
		}

		if entry.Metric.exact == nil {
			exactBuilder.AppendNull()
		}

		supportBuilder.Append(1.0)
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
