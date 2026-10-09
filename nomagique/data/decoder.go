package data

import (
	"fmt"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

/*
Decode populates the Measurement from row rowIdx of the Arrow RecordBatch.
*/
func (measurement *Measurement) Decode(batch arrow.RecordBatch, rowIdx int) error {
	if measurement == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[data.decoder] measurement is nil",
			nil,
		))
	}

	cols := make(map[string]arrow.Array, batch.NumCols())

	for colIdx := range int(batch.NumCols()) {
		cols[batch.ColumnName(colIdx)] = batch.Column(colIdx)
	}

	epochCol, _ := cols["epoch"].(*array.Int64)
	seqIdxCol, _ := cols["seqIdx"].(*array.Int64)
	sourceCol, _ := cols["source"].(*array.String)
	labelCol, _ := cols["label"].(*array.String)
	tickCol, _ := cols["tick"].(*array.Int64)
	atCol, _ := cols["at"].(*array.Timestamp)
	fromCol, _ := cols["from"].(*array.Timestamp)
	timestampCol, _ := cols["timestamp"].(*array.Int64)
	idCol, _ := cols["id"].(*array.Int64)
	coherenceCol, _ := cols["coherence"].(*array.Float64)
	maturityCol, _ := cols["maturity"].(*array.Float64)
	samplesCol, _ := cols["samples"].(*array.Int64)
	energyCol, _ := cols["energy"].(*array.Float64)
	predictionCol, _ := cols["prediction"].(*array.Float64)
	metricsCol, _ := cols["metrics"].(*array.Map)
	metadataCol, _ := cols["metadata"].(*array.Map)

	confident := maturityCol != nil && coherenceCol != nil

	if !confident && (maturityCol != nil || coherenceCol != nil) {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"iceberg: measurement batch carries only one of maturity/coherence",
			nil,
		))
	}

	if epochCol != nil && !epochCol.IsNull(rowIdx) {
		measurement.Epoch = epochCol.Value(rowIdx)
	}

	if seqIdxCol != nil && !seqIdxCol.IsNull(rowIdx) {
		measurement.SeqIdx = seqIdxCol.Value(rowIdx)
	}

	if tickCol != nil && !tickCol.IsNull(rowIdx) {
		measurement.Tick = tickCol.Value(rowIdx)
	}

	if sourceCol != nil && !sourceCol.IsNull(rowIdx) {
		measurement.Source = sourceCol.Value(rowIdx)
	}

	if labelCol != nil && !labelCol.IsNull(rowIdx) {
		measurement.Label = labelCol.Value(rowIdx)
	}

	if idCol != nil && !idCol.IsNull(rowIdx) {
		measurement.ID = uint32(idCol.Value(rowIdx))
	}

	if samplesCol != nil && !samplesCol.IsNull(rowIdx) {
		measurement.samples = samplesCol.Value(rowIdx)
	}

	if energyCol != nil && !energyCol.IsNull(rowIdx) {
		measurement.energy = energyCol.Value(rowIdx)
	}

	if predictionCol != nil && !predictionCol.IsNull(rowIdx) {
		measurement.prediction = predictionCol.Value(rowIdx)
	}

	if metadataCol != nil && !metadataCol.IsNull(rowIdx) {
		keyArray := metadataCol.Keys().(*array.String)
		valArray := metadataCol.Items().(*array.String)
		offsets := metadataCol.Offsets()
		startOffset := int(offsets[rowIdx])
		endOffset := int(offsets[rowIdx+1])

		measurement.metadata = make([]*StringEntry, 0, endOffset-startOffset)

		for itemIdx := startOffset; itemIdx < endOffset; itemIdx++ {
			measurement.metadata = append(measurement.metadata, &StringEntry{
				Key:   keyArray.Value(itemIdx),
				Value: valArray.Value(itemIdx),
			})
		}
	}

	if confident && (maturityCol.IsNull(rowIdx) || coherenceCol.IsNull(rowIdx)) {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("iceberg: measurement row %d has no maturity/coherence", rowIdx),
			nil,
		))
	}

	if confident {
		measurement.coherence = coherenceCol.Value(rowIdx)
		measurement.maturity = maturityCol.Value(rowIdx)
	}

	if atCol != nil && !atCol.IsNull(rowIdx) {
		measurement.At = time.UnixMicro(int64(atCol.Value(rowIdx))).UTC()
		measurement.Timestamp = int64(atCol.Value(rowIdx)) * 1000
	}

	if fromCol != nil && !fromCol.IsNull(rowIdx) {
		measurement.From = time.UnixMicro(int64(fromCol.Value(rowIdx))).UTC()
	}

	if fromCol == nil && atCol != nil && !atCol.IsNull(rowIdx) {
		measurement.From = measurement.At
	}

	if timestampCol != nil && !timestampCol.IsNull(rowIdx) {
		measurement.Timestamp = timestampCol.Value(rowIdx)
	}

	if metricsCol != nil && !metricsCol.IsNull(rowIdx) {
		keyArray := metricsCol.Keys().(*array.String)
		offsets := metricsCol.Offsets()
		startOffset := int(offsets[rowIdx])
		endOffset := int(offsets[rowIdx+1])

		measurement.metrics = make([]*MetricEntry, 0, endOffset-startOffset)

		if itemStruct, ok := metricsCol.Items().(*array.Struct); ok {
			rawCol := itemStruct.Field(0).(*array.Float64)
			normCol := itemStruct.Field(1).(*array.Float64)
			stdCol := itemStruct.Field(2).(*array.Float64)
			exactCol := itemStruct.Field(3).(*array.String)
			centerCol := itemStruct.Field(4).(*array.Float64)
			scaleCol := itemStruct.Field(5).(*array.Float64)
			unitCol := itemStruct.Field(6).(*array.String)
			timeCol := itemStruct.Field(7).(*array.String)

			for itemIdx := startOffset; itemIdx < endOffset; itemIdx++ {
				metricKey := keyArray.Value(itemIdx)
				rawVal := rawCol.Value(itemIdx)
				unitVal := CanonicalUnit(metricKey, Unit(unitCol.Value(itemIdx)))
				timeVal := CanonicalTimescale(metricKey, Timescale(timeCol.Value(itemIdx)))

				metric := NewMetric(metricKey, rawVal, unitVal, timeVal)
				metric.Normalized = normCol.Value(itemIdx)
				metric.Standardized = stdCol.Value(itemIdx)
				metric.center = centerCol.Value(itemIdx)
				metric.scale = scaleCol.Value(itemIdx)

				if exactCol != nil && !exactCol.IsNull(itemIdx) && exactCol.Value(itemIdx) != "" {
					exactDec, err := decimal.NewFromString(exactCol.Value(itemIdx))

					if err != nil {
						return errnie.Error(errnie.Err(
							errnie.Validation,
							fmt.Sprintf("iceberg: measurement row %d metric %s has an unparseable exact value", rowIdx, metricKey),
							err,
						))
					}

					metric.Exact = exactDec
				}

				measurement.metrics = append(measurement.metrics, &MetricEntry{
					Key:    metricKey,
					Metric: metric,
				})
			}
		} else if floatItems, ok := metricsCol.Items().(*array.Float64); ok {
			for itemIdx := startOffset; itemIdx < endOffset; itemIdx++ {
				metricKey := keyArray.Value(itemIdx)
				rawVal := floatItems.Value(itemIdx)

				metric := NewMetric(metricKey, rawVal, UnitDimensionless, TimescaleInstantaneous)
				metric.center = rawVal

				measurement.metrics = append(measurement.metrics, &MetricEntry{
					Key:    metricKey,
					Metric: metric,
				})
			}
		}
	}

	measurement.valid()

	if err := measurement.Error(); confident && err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"iceberg: stored measurement row %d is invalid (epoch %d, source %s, label %s, tick %d)",
				rowIdx, measurement.Epoch, measurement.Source, measurement.Label, measurement.Tick,
			),
			err,
		))
	}

	return nil
}

/*
DecodeMeasurement creates and decodes a Measurement from an Arrow RecordBatch row.
*/
func DecodeMeasurement(batch arrow.RecordBatch, rowIdx int) (*Measurement, error) {
	measurement := NewMeasurement(0, "", "", 0, 0)

	if err := measurement.Decode(batch, rowIdx); err != nil {
		return nil, err
	}

	return measurement, nil
}

/*
DecodeBatch decodes an entire Arrow RecordBatch into a slice of Measurements.
*/
func DecodeBatch(batch arrow.RecordBatch) ([]*Measurement, error) {
	totalRows := int(batch.NumRows())
	measurements := make([]*Measurement, 0, totalRows)

	for rowIdx := range totalRows {
		measurement, err := DecodeMeasurement(batch, rowIdx)

		if err != nil {
			return nil, err
		}

		measurements = append(measurements, measurement)
	}

	return measurements, nil
}
