package data

import (
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
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

	epochBuilder := recordBuilder.Field(0).(*array.Int64Builder)
	seqIdxBuilder := recordBuilder.Field(1).(*array.Int64Builder)
	sourceBuilder := recordBuilder.Field(2).(*array.StringBuilder)
	labelBuilder := recordBuilder.Field(3).(*array.StringBuilder)
	tickBuilder := recordBuilder.Field(4).(*array.Int64Builder)
	atBuilder := recordBuilder.Field(5).(*array.TimestampBuilder)
	fromBuilder := recordBuilder.Field(6).(*array.TimestampBuilder)
	timestampBuilder := recordBuilder.Field(7).(*array.Int64Builder)
	idBuilder := recordBuilder.Field(8).(*array.Int64Builder)
	coherenceBuilder := recordBuilder.Field(9).(*array.Float64Builder)
	maturityBuilder := recordBuilder.Field(10).(*array.Float64Builder)
	samplesBuilder := recordBuilder.Field(11).(*array.Int64Builder)
	energyBuilder := recordBuilder.Field(12).(*array.Float64Builder)
	predictionBuilder := recordBuilder.Field(13).(*array.Float64Builder)
	metricsBuilder := recordBuilder.Field(14).(*array.MapBuilder)
	metadataBuilder := recordBuilder.Field(15).(*array.MapBuilder)

	epochBuilder.Append(recEpoch)
	seqIdxBuilder.Append(measurement.SeqIdx)
	sourceBuilder.Append(measurement.Source)
	labelBuilder.Append(measurement.Label)
	tickBuilder.Append(measurement.Tick)
	atBuilder.Append(arrow.Timestamp(measurement.At.UTC().UnixMicro()))
	fromBuilder.Append(arrow.Timestamp(measurement.From.UTC().UnixMicro()))
	timestampBuilder.Append(measurement.Timestamp)
	idBuilder.Append(int64(measurement.ID))
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
	centerBuilder := itemBuilder.FieldBuilder(4).(*array.Float64Builder)
	scaleBuilder := itemBuilder.FieldBuilder(5).(*array.Float64Builder)
	unitBuilder := itemBuilder.FieldBuilder(6).(*array.StringBuilder)
	timescaleBuilder := itemBuilder.FieldBuilder(7).(*array.StringBuilder)

	metricsBuilder.Append(true)

	for _, entry := range measurement.metrics {
		if entry == nil || entry.Metric == nil {
			continue
		}

		metricsKey.Append(entry.Key)
		itemBuilder.Append(true)
		rawBuilder.Append(entry.Metric.Raw)
		normBuilder.Append(entry.Metric.Normalized)
		stdBuilder.Append(entry.Metric.Standardized)

		if entry.Metric.Exact != nil {
			exactBuilder.Append(entry.Metric.Exact.String())
		} else {
			exactBuilder.AppendNull()
		}

		centerBuilder.Append(entry.Metric.center)
		scaleBuilder.Append(entry.Metric.scale)
		unitBuilder.Append(string(entry.Metric.unit))
		timescaleBuilder.Append(string(entry.Metric.timescale))
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
