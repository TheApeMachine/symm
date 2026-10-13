package data

import (
	"fmt"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

type batchCols struct {
	epochCol      *array.Int64
	seqIdxCol     *array.Int64
	sourceCol     *array.String
	labelCol      *array.String
	tickCol       *array.Int64
	atCol         *array.Timestamp
	fromCol       *array.Timestamp
	timestampCol  *array.Int64
	idCol         *array.Int64
	maturityCol   *array.Float64
	samplesCol    *array.Int64
	energyCol     *array.Float64
	predictionCol *array.Float64
	metricsCol    *array.Map
	metadataCol   *array.Map

	metaKeyArray *array.String
	metaValArray *array.String
	metaOffsets  []int32

	metricsKeyArray *array.String
	metricsOffsets  []int32
	rawCol          *array.Float64
	snmCol          *array.Float64
	exactCol        *array.String
	unitCol         *array.String
	timeCol         *array.String
	floatItems      *array.Float64
}

func newBatchCols(batch arrow.RecordBatch) (*batchCols, error) {
	cols := &batchCols{}

	for colIdx := range int(batch.NumCols()) {
		col := batch.Column(colIdx)

		switch batch.ColumnName(colIdx) {
		case "epoch":
			var ok bool
			if cols.epochCol, ok = col.(*array.Int64); !ok {
				return nil, errnie.Error(errnie.Err(errnie.Validation, "iceberg: column epoch is not *array.Int64", nil))
			}
		case "seqIdx", "seq_idx":
			var ok bool
			if cols.seqIdxCol, ok = col.(*array.Int64); !ok {
				return nil, errnie.Error(errnie.Err(errnie.Validation, "iceberg: column seq_idx is not *array.Int64", nil))
			}
		case "source":
			var ok bool
			if cols.sourceCol, ok = col.(*array.String); !ok {
				return nil, errnie.Error(errnie.Err(errnie.Validation, "iceberg: column source is not *array.String", nil))
			}
		case "label":
			var ok bool
			if cols.labelCol, ok = col.(*array.String); !ok {
				return nil, errnie.Error(errnie.Err(errnie.Validation, "iceberg: column label is not *array.String", nil))
			}
		case "tick":
			var ok bool
			if cols.tickCol, ok = col.(*array.Int64); !ok {
				return nil, errnie.Error(errnie.Err(errnie.Validation, "iceberg: column tick is not *array.Int64", nil))
			}
		case "at":
			var ok bool
			if cols.atCol, ok = col.(*array.Timestamp); !ok {
				return nil, errnie.Error(errnie.Err(errnie.Validation, "iceberg: column at is not *array.Timestamp", nil))
			}
		case "from":
			var ok bool
			if cols.fromCol, ok = col.(*array.Timestamp); !ok {
				return nil, errnie.Error(errnie.Err(errnie.Validation, "iceberg: column from is not *array.Timestamp", nil))
			}
		case "timestamp":
			var ok bool
			if cols.timestampCol, ok = col.(*array.Int64); !ok {
				return nil, errnie.Error(errnie.Err(errnie.Validation, "iceberg: column timestamp is not *array.Int64", nil))
			}
		case "id":
			var ok bool
			if cols.idCol, ok = col.(*array.Int64); !ok {
				return nil, errnie.Error(errnie.Err(errnie.Validation, "iceberg: column id is not *array.Int64", nil))
			}
		case "maturity":
			var ok bool
			if cols.maturityCol, ok = col.(*array.Float64); !ok {
				return nil, errnie.Error(errnie.Err(errnie.Validation, "iceberg: column maturity is not *array.Float64", nil))
			}
		case "samples":
			var ok bool
			if cols.samplesCol, ok = col.(*array.Int64); !ok {
				return nil, errnie.Error(errnie.Err(errnie.Validation, "iceberg: column samples is not *array.Int64", nil))
			}
		case "energy":
			var ok bool
			if cols.energyCol, ok = col.(*array.Float64); !ok {
				return nil, errnie.Error(errnie.Err(errnie.Validation, "iceberg: column energy is not *array.Float64", nil))
			}
		case "prediction":
			var ok bool
			if cols.predictionCol, ok = col.(*array.Float64); !ok {
				return nil, errnie.Error(errnie.Err(errnie.Validation, "iceberg: column prediction is not *array.Float64", nil))
			}
		case "metrics":
			var ok bool
			if cols.metricsCol, ok = col.(*array.Map); !ok {
				return nil, errnie.Error(errnie.Err(errnie.Validation, "iceberg: column metrics is not *array.Map", nil))
			}
		case "metadata":
			var ok bool
			if cols.metadataCol, ok = col.(*array.Map); !ok {
				return nil, errnie.Error(errnie.Err(errnie.Validation, "iceberg: column metadata is not *array.Map", nil))
			}
		}
	}

	if cols.metadataCol != nil {
		var ok bool
		if cols.metaKeyArray, ok = cols.metadataCol.Keys().(*array.String); !ok {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "iceberg: metadata keys is not *array.String", nil))
		}
		if cols.metaValArray, ok = cols.metadataCol.Items().(*array.String); !ok {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "iceberg: metadata items is not *array.String", nil))
		}
		cols.metaOffsets = cols.metadataCol.Offsets()
	}

	if cols.metricsCol != nil {
		var ok bool
		if cols.metricsKeyArray, ok = cols.metricsCol.Keys().(*array.String); !ok {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "iceberg: metrics keys is not *array.String", nil))
		}
		cols.metricsOffsets = cols.metricsCol.Offsets()

		if itemStruct, ok := cols.metricsCol.Items().(*array.Struct); ok {
			cols.rawCol, _ = itemStruct.Field(0).(*array.Float64)
			cols.snmCol, _ = itemStruct.Field(1).(*array.Float64)
			cols.exactCol, _ = itemStruct.Field(2).(*array.String)
			cols.unitCol, _ = itemStruct.Field(3).(*array.String)
			cols.timeCol, _ = itemStruct.Field(4).(*array.String)
		} else if floatItems, ok := cols.metricsCol.Items().(*array.Float64); ok {
			cols.floatItems = floatItems
		} else {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "iceberg: metrics items is neither *array.Struct nor *array.Float64", nil))
		}
	}

	return cols, nil
}

func (cols *batchCols) decodeRow(measurement *Measurement, rowIdx int) error {
	if cols.epochCol != nil && !cols.epochCol.IsNull(rowIdx) {
		measurement.Epoch = cols.epochCol.Value(rowIdx)
	}

	if cols.seqIdxCol != nil && !cols.seqIdxCol.IsNull(rowIdx) {
		measurement.SeqIdx = cols.seqIdxCol.Value(rowIdx)
	}

	if cols.tickCol != nil && !cols.tickCol.IsNull(rowIdx) {
		measurement.Tick = cols.tickCol.Value(rowIdx)
	}

	if cols.sourceCol != nil && !cols.sourceCol.IsNull(rowIdx) {
		measurement.Source = cols.sourceCol.Value(rowIdx)
	}

	if cols.labelCol != nil && !cols.labelCol.IsNull(rowIdx) {
		measurement.Label = cols.labelCol.Value(rowIdx)
	}

	if cols.idCol != nil && !cols.idCol.IsNull(rowIdx) {
		measurement.ID = uint32(cols.idCol.Value(rowIdx))
	}

	if cols.samplesCol != nil && !cols.samplesCol.IsNull(rowIdx) {
		measurement.samples = cols.samplesCol.Value(rowIdx)
	}

	if cols.energyCol != nil && !cols.energyCol.IsNull(rowIdx) {
		measurement.energy = cols.energyCol.Value(rowIdx)
	}

	if cols.predictionCol != nil && !cols.predictionCol.IsNull(rowIdx) {
		measurement.prediction = cols.predictionCol.Value(rowIdx)
	}

	if cols.metaOffsets != nil && cols.metaKeyArray != nil && cols.metaValArray != nil && !cols.metadataCol.IsNull(rowIdx) {
		startOffset := int(cols.metaOffsets[rowIdx])
		endOffset := int(cols.metaOffsets[rowIdx+1])

		measurement.metadata = make([]*StringEntry, 0, endOffset-startOffset)

		for itemIdx := startOffset; itemIdx < endOffset; itemIdx++ {
			measurement.metadata = append(measurement.metadata, &StringEntry{
				Key:   cols.metaKeyArray.Value(itemIdx),
				Value: cols.metaValArray.Value(itemIdx),
			})
		}
	}

	if cols.maturityCol != nil && !cols.maturityCol.IsNull(rowIdx) {
		measurement.maturity = cols.maturityCol.Value(rowIdx)
	}

	if cols.atCol != nil && !cols.atCol.IsNull(rowIdx) {
		measurement.At = time.UnixMicro(int64(cols.atCol.Value(rowIdx))).UTC()
		measurement.Timestamp = int64(cols.atCol.Value(rowIdx)) * 1000
	}

	if cols.fromCol != nil && !cols.fromCol.IsNull(rowIdx) {
		measurement.From = time.UnixMicro(int64(cols.fromCol.Value(rowIdx))).UTC()
	}

	if cols.fromCol == nil && cols.atCol != nil && !cols.atCol.IsNull(rowIdx) {
		measurement.From = measurement.At
	}

	if cols.timestampCol != nil && !cols.timestampCol.IsNull(rowIdx) {
		measurement.Timestamp = cols.timestampCol.Value(rowIdx)
	}

	if cols.metricsOffsets != nil && cols.metricsKeyArray != nil && !cols.metricsCol.IsNull(rowIdx) {
		startOffset := int(cols.metricsOffsets[rowIdx])
		endOffset := int(cols.metricsOffsets[rowIdx+1])

		measurement.metrics = make([]*MetricEntry, 0, endOffset-startOffset)

		if cols.rawCol != nil {
			for itemIdx := startOffset; itemIdx < endOffset; itemIdx++ {
				metricKey := cols.metricsKeyArray.Value(itemIdx)
				rawVal := cols.rawCol.Value(itemIdx)
				var unitVal Unit
				var timeVal Timescale

				if cols.unitCol != nil && !cols.unitCol.IsNull(itemIdx) {
					unitVal = Unit(cols.unitCol.Value(itemIdx))
				}

				if cols.timeCol != nil && !cols.timeCol.IsNull(itemIdx) {
					timeVal = Timescale(cols.timeCol.Value(itemIdx))
				}

				if unitVal == "" || timeVal == "" {
					u, t := GetDimension(measurement.Source, metricKey)

					if unitVal == "" {
						unitVal = u
					}

					if timeVal == "" {
						timeVal = t
					}
				}

				metric := NewMetric(metricKey, rawVal, unitVal, timeVal)

				if cols.snmCol != nil && !cols.snmCol.IsNull(itemIdx) {
					metric.snm = cols.snmCol.Value(itemIdx)
				}

				if cols.exactCol != nil && !cols.exactCol.IsNull(itemIdx) && cols.exactCol.Value(itemIdx) != "" {
					exactDec, err := decimal.NewFromString(cols.exactCol.Value(itemIdx))

					if err != nil {
						return errnie.Error(errnie.Err(
							errnie.Validation,
							fmt.Sprintf("iceberg: measurement row %d metric %s has an unparseable exact value", rowIdx, metricKey),
							err,
						))
					}

					metric.exact = exactDec
				}

				measurement.metrics = append(measurement.metrics, &MetricEntry{
					Key:    metricKey,
					Metric: metric,
				})
			}
		}

		if cols.floatItems != nil {
			for itemIdx := startOffset; itemIdx < endOffset; itemIdx++ {
				metricKey := cols.metricsKeyArray.Value(itemIdx)
				rawVal := cols.floatItems.Value(itemIdx)

				metric := NewMetric(metricKey, rawVal, UnitDimensionless, TimescaleInstantaneous)

				measurement.metrics = append(measurement.metrics, &MetricEntry{
					Key:    metricKey,
					Metric: metric,
				})
			}
		}
	}

	measurement.valid()

	if err := measurement.Error(); err != nil {
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

	cols, err := newBatchCols(batch)

	if err != nil {
		return err
	}

	return cols.decodeRow(measurement, rowIdx)
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
	cols, err := newBatchCols(batch)

	if err != nil {
		return nil, err
	}

	totalRows := int(batch.NumRows())
	measurements := make([]*Measurement, 0, totalRows)

	for rowIdx := range totalRows {
		measurement := NewMeasurement(0, "", "", 0, 0)

		if err := cols.decodeRow(measurement, rowIdx); err != nil {
			return nil, err
		}

		measurements = append(measurements, measurement)
	}

	return measurements, nil
}

/*
FromWire decodes a wire.MeasurementT into a domain Measurement.
*/
func FromWire(row *wire.MeasurementT) *Measurement {
	if row == nil {
		return nil
	}

	meas := NewMeasurement(
		0,
		row.Symbol,
		row.Source,
		row.Tick,
		row.Tick,
	)

	meas.At = time.Unix(0, row.At).UTC()
	meas.From = time.Unix(0, row.ObservedFrom).UTC()
	meas.maturity = row.Maturity

	for _, metric := range row.Metrics {
		if metric == nil {
			continue
		}
		unit, timescale := GetDimension(row.Source, metric.Name)
		item := NewMetric(metric.Name, metric.Raw, unit, timescale)
		item.snm = metric.Snm
		meas.metrics = append(meas.metrics, &MetricEntry{
			Key:    metric.Name,
			Metric: item,
		})
	}

	for _, peer := range row.Peers {
		if peer == nil {
			continue
		}
		if wirePeer := FromWire(peer); wirePeer != nil {
			meas.peers = append(meas.peers, wirePeer)
		}
	}

	meas.finalize()
	return meas
}

