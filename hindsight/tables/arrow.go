package tables

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/iceberg-go"
	icetable "github.com/apache/iceberg-go/table"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
arrowSchemaFor converts an Iceberg schema to an Arrow schema preserving field IDs in metadata.
*/
func arrowSchemaFor(schema *iceberg.Schema) (*arrow.Schema, error) {
	converted, err := icetable.SchemaToArrowSchema(schema, nil, true, false)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"[iceberg] failed to convert schema to arrow",
			err,
		))
	}

	return converted, nil
}

func fillMeasurements(
	recordBuilder *array.RecordBuilder,
	measurements []*data.Measurement[float64],
	epoch int64,
) {
	epochBuilder := recordBuilder.Field(0).(*array.Int64Builder)
	seqIdxBuilder := recordBuilder.Field(1).(*array.Int64Builder)
	sourceBuilder := recordBuilder.Field(2).(*array.StringBuilder)
	labelBuilder := recordBuilder.Field(3).(*array.StringBuilder)
	atBuilder := recordBuilder.Field(4).(*array.TimestampBuilder)
	maturityBuilder := recordBuilder.Field(5).(*array.Float64Builder)
	snrBuilder := recordBuilder.Field(6).(*array.Float64Builder)
	snrDefinedBuilder := recordBuilder.Field(7).(*array.BooleanBuilder)
	metricsBuilder := recordBuilder.Field(8).(*array.MapBuilder)
	metadataBuilder := recordBuilder.Field(9).(*array.MapBuilder)
	provenanceBuilder := recordBuilder.Field(10).(*array.MapBuilder)
	tickBuilder := recordBuilder.Field(11).(*array.Int64Builder)

	metricsKey := metricsBuilder.KeyBuilder().(*array.StringBuilder)
	metricsVal := metricsBuilder.ItemBuilder().(*array.Float64Builder)

	metadataKey := metadataBuilder.KeyBuilder().(*array.StringBuilder)
	metadataVal := metadataBuilder.ItemBuilder().(*array.StringBuilder)

	provenanceKey := provenanceBuilder.KeyBuilder().(*array.StringBuilder)
	provenanceVal := provenanceBuilder.ItemBuilder().(*array.StringBuilder)

	for _, measurement := range measurements {
		recEpoch := measurement.Epoch
		if recEpoch <= 0 {
			recEpoch = epoch
		}
		epochBuilder.Append(recEpoch)
		seqIdxBuilder.Append(measurement.SeqIdx)
		sourceBuilder.Append(measurement.Source)
		labelBuilder.Append(measurement.Label)
		atBuilder.Append(arrow.Timestamp(measurement.At.UTC().UnixMicro()))
		maturityBuilder.Append(measurement.Maturity)
		snrBuilder.Append(measurement.SNR)
		snrDefinedBuilder.Append(measurement.SNRDefined)
		tickBuilder.Append(measurement.Tick)

		metricsBuilder.Append(true)
		measurement.RangeMetrics(func(metricKey string, metricVal data.Metric[float64]) bool {
			metricsKey.Append(metricKey)
			metricsVal.Append(metricVal.Raw)
			return true
		})

		metadataBuilder.Append(true)
		for _, entry := range measurement.Metadata {
			metadataKey.Append(entry.Key)
			metadataVal.Append(entry.Value)
		}

		provenanceBuilder.Append(true)
		for _, entry := range measurement.Provenance {
			provenanceKey.Append(entry.Key)
			provenanceVal.Append(entry.Value)
		}

		if measurement.Err != nil {
			provenanceKey.Append("symm:error")
			provenanceVal.Append(fmt.Sprintf("%v", measurement.Err))
		}
		provenanceKey.Append("symm:estimated")
		provenanceVal.Append(strconv.FormatBool(measurement.Estimated))
		measurement.RangeMetrics(func(key string, metric data.Metric[float64]) bool {
			if metric.Exact != nil {
				provenanceKey.Append("symm:exact:" + key)
				provenanceVal.Append(metric.Exact.String())
			}

			if metric.Standardized != nil {
				provenanceKey.Append("symm:standardized:" + key)
				provenanceVal.Append(strconv.FormatFloat(*metric.Standardized, 'g', -1, 64))
			}

			if metric.Normalized != nil {
				provenanceKey.Append("symm:normalized:" + key)
				provenanceVal.Append(strconv.FormatFloat(*metric.Normalized, 'g', -1, 64))
			}
			return true
		})
	}
}

func ReadMeasurements(batch arrow.RecordBatch) ([]*data.Measurement[float64], error) {
	totalRows := int(batch.NumRows())
	measurements := make([]*data.Measurement[float64], 0, totalRows)

	cols := make(map[string]arrow.Array, batch.NumCols())

	for colIdx := range int(batch.NumCols()) {
		cols[batch.ColumnName(colIdx)] = batch.Column(colIdx)
	}

	epochCol, _ := cols["epoch"].(*array.Int64)
	seqIdxCol, _ := cols["seqIdx"].(*array.Int64)
	sourceCol, _ := cols["source"].(*array.String)
	labelCol, _ := cols["label"].(*array.String)
	atCol, _ := cols["at"].(*array.Timestamp)
	maturityCol, _ := cols["maturity"].(*array.Float64)
	snrCol, _ := cols["snr"].(*array.Float64)
	snrDefinedCol, _ := cols["snrDefined"].(*array.Boolean)
	metricsCol, _ := cols["metrics"].(*array.Map)
	metadataCol, _ := cols["metadata"].(*array.Map)
	provenanceCol, _ := cols["provenance"].(*array.Map)
	tickCol, _ := cols["tick"].(*array.Int64)

	for rowIdx := range totalRows {
		source := ""

		if sourceCol != nil && !sourceCol.IsNull(rowIdx) {
			source = sourceCol.Value(rowIdx)
		}

		measurement := data.NewMeasurement[float64](source, nil)

		if epochCol != nil && !epochCol.IsNull(rowIdx) {
			measurement.Epoch = epochCol.Value(rowIdx)
		}

		if seqIdxCol != nil && !seqIdxCol.IsNull(rowIdx) {
			measurement.SeqIdx = seqIdxCol.Value(rowIdx)
		}

		if tickCol != nil && !tickCol.IsNull(rowIdx) {
			measurement.Tick = tickCol.Value(rowIdx)
		}

		if labelCol != nil && !labelCol.IsNull(rowIdx) {
			measurement.Label = labelCol.Value(rowIdx)
		}

		if atCol != nil && !atCol.IsNull(rowIdx) {
			measurement.At = time.UnixMicro(int64(atCol.Value(rowIdx))).UTC()
		}

		if maturityCol != nil && !maturityCol.IsNull(rowIdx) {
			measurement.Maturity = maturityCol.Value(rowIdx)
		}

		if snrDefinedCol != nil && !snrDefinedCol.IsNull(rowIdx) {
			measurement.SNRDefined = snrDefinedCol.Value(rowIdx)
		}

		if snrCol != nil && !snrCol.IsNull(rowIdx) && measurement.SNRDefined {
			measurement.SNR = snrCol.Value(rowIdx)
		}

		if metricsCol != nil && !metricsCol.IsNull(rowIdx) {
			keyArray := metricsCol.Keys().(*array.String)
			valArray := metricsCol.Items().(*array.Float64)
			offsets := metricsCol.Offsets()
			startOffset := int(offsets[rowIdx])
			endOffset := int(offsets[rowIdx+1])

			for itemIdx := startOffset; itemIdx < endOffset; itemIdx++ {
				metricKey := keyArray.Value(itemIdx)
				metricVal := valArray.Value(itemIdx)
				measurement.SetMetric(metricKey, data.Metric[float64]{
					Label: metricKey,
					Raw:   metricVal,
				})
			}
		}

		if metadataCol != nil && !metadataCol.IsNull(rowIdx) {
			keyArray := metadataCol.Keys().(*array.String)
			valArray := metadataCol.Items().(*array.String)
			offsets := metadataCol.Offsets()
			startOffset := int(offsets[rowIdx])
			endOffset := int(offsets[rowIdx+1])

			for itemIdx := startOffset; itemIdx < endOffset; itemIdx++ {
				measurement.SetMetadata(keyArray.Value(itemIdx), valArray.Value(itemIdx))
			}
		}

		if provenanceCol != nil && !provenanceCol.IsNull(rowIdx) {
			keyArray := provenanceCol.Keys().(*array.String)
			valArray := provenanceCol.Items().(*array.String)
			offsets := provenanceCol.Offsets()
			startOffset := int(offsets[rowIdx])
			endOffset := int(offsets[rowIdx+1])

			for itemIdx := startOffset; itemIdx < endOffset; itemIdx++ {
				key, value := keyArray.Value(itemIdx), valArray.Value(itemIdx)
				if key == "symm:error" {
					measurement.Err = errors.New(value)
					continue
				}

				if key == "symm:estimated" {
					measurement.Estimated = value == "true"
					continue
				}

				if after, ok :=strings.CutPrefix(key, "symm:exact:"); ok  {
					metricKey := after
					exact, err := decimal.NewFromString(value)

					if err != nil {
						return nil, errnie.Error(errnie.Err(errnie.Validation, "iceberg: invalid original decimal quantity", err))
					}

					metric := measurement.GetMetric(metricKey)
					metric.Exact = exact
					measurement.SetMetric(metricKey, metric)
					continue
				}

				if after, ok :=strings.CutPrefix(key, "symm:standardized:"); ok  {
					metricKey := after
					parsed, err := strconv.ParseFloat(value, 64)

					if err != nil {
						return nil, errnie.Error(errnie.Err(errnie.Validation, "iceberg: invalid standardized metric", err))
					}

					metric := measurement.GetMetric(metricKey)
					metric.Standardized = &parsed
					measurement.SetMetric(metricKey, metric)
					continue
				}

				if after, ok :=strings.CutPrefix(key, "symm:normalized:"); ok  {
					metricKey := after
					parsed, err := strconv.ParseFloat(value, 64)

					if err != nil {
						return nil, errnie.Error(errnie.Err(errnie.Validation, "iceberg: invalid normalized metric", err))
					}

					metric := measurement.GetMetric(metricKey)
					metric.Normalized = &parsed
					measurement.SetMetric(metricKey, metric)
					continue
				}

				measurement.SetProvenance(key, value)
			}
		}

		measurements = append(measurements, measurement)
	}

	return measurements, nil
}

func fillRuns(recordBuilder *array.RecordBuilder, runs []Run) {
	epochBuilder := recordBuilder.Field(0).(*array.Int64Builder)
	startedAtBuilder := recordBuilder.Field(1).(*array.TimestampBuilder)
	codeCommitBuilder := recordBuilder.Field(2).(*array.StringBuilder)
	buildIDBuilder := recordBuilder.Field(3).(*array.StringBuilder)
	configDigestBuilder := recordBuilder.Field(4).(*array.StringBuilder)
	statusBuilder := recordBuilder.Field(5).(*array.StringBuilder)

	for _, run := range runs {
		epochBuilder.Append(run.Epoch)
		startedAtBuilder.Append(arrow.Timestamp(run.StartedAt.UTC().UnixMicro()))
		codeCommitBuilder.Append(run.CodeCommit)
		buildIDBuilder.Append(run.BuildID)
		configDigestBuilder.Append(run.ConfigDigest)
		statusBuilder.Append(run.Status)
	}
}

func readRuns(batch arrow.RecordBatch) []Run {
	totalRows := int(batch.NumRows())
	runs := make([]Run, 0, totalRows)

	cols := make(map[string]arrow.Array, batch.NumCols())

	for colIdx := range int(batch.NumCols()) {
		cols[batch.ColumnName(colIdx)] = batch.Column(colIdx)
	}

	epochCol, _ := cols["epoch"].(*array.Int64)
	startedAtCol, _ := cols["started_at"].(*array.Timestamp)
	codeCommitCol, _ := cols["code_commit"].(*array.String)
	buildIDCol, _ := cols["build_id"].(*array.String)
	configDigestCol, _ := cols["config_digest"].(*array.String)
	statusCol, _ := cols["status"].(*array.String)

	for rowIdx := range totalRows {
		run := Run{}

		if epochCol != nil && !epochCol.IsNull(rowIdx) {
			run.Epoch = epochCol.Value(rowIdx)
		}

		if startedAtCol != nil && !startedAtCol.IsNull(rowIdx) {
			run.StartedAt = time.UnixMicro(int64(startedAtCol.Value(rowIdx))).UTC()
		}

		if codeCommitCol != nil && !codeCommitCol.IsNull(rowIdx) {
			run.CodeCommit = codeCommitCol.Value(rowIdx)
		}

		if buildIDCol != nil && !buildIDCol.IsNull(rowIdx) {
			run.BuildID = buildIDCol.Value(rowIdx)
		}

		if configDigestCol != nil && !configDigestCol.IsNull(rowIdx) {
			run.ConfigDigest = configDigestCol.Value(rowIdx)
		}

		if statusCol != nil && !statusCol.IsNull(rowIdx) {
			run.Status = statusCol.Value(rowIdx)
		}

		runs = append(runs, run)
	}

	return runs
}

func measurementRecords(
	schema *iceberg.Schema,
	measurements []*data.Measurement[float64],
	epoch int64,
) (array.RecordReader, error) {
	if len(measurements) == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[iceberg] measurementRecords requires rows",
			nil,
		))
	}

	converted, err := arrowSchemaFor(schema)

	if err != nil {
		return nil, err
	}

	recordBuilder := array.NewRecordBuilder(memory.DefaultAllocator, converted)
	defer recordBuilder.Release()

	recordBuilder.Reserve(len(measurements))
	fillMeasurements(recordBuilder, measurements, epoch)
	batch := recordBuilder.NewRecordBatch()
	defer batch.Release()

	reader, err := array.NewRecordReader(converted, []arrow.RecordBatch{batch})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"[iceberg] failed to build measurement record reader",
			err,
		))
	}

	return reader, nil
}

func runRecords(schema *iceberg.Schema, runs []Run) (array.RecordReader, error) {
	converted, err := arrowSchemaFor(schema)

	if err != nil {
		return nil, err
	}

	recordBuilder := array.NewRecordBuilder(memory.DefaultAllocator, converted)
	defer recordBuilder.Release()

	recordBuilder.Reserve(len(runs))
	fillRuns(recordBuilder, runs)
	batch := recordBuilder.NewRecordBatch()
	defer batch.Release()

	reader, err := array.NewRecordReader(converted, []arrow.RecordBatch{batch})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"[iceberg] failed to build run record reader",
			err,
		))
	}

	return reader, nil
}


