package tables

import (
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

type metricHolder struct {
	raw             float64
	exact           *decimal.Decimal
	standardized    float64
	hasStandardized bool
	normalized      float64
	hasNormalized   bool
}

func fillMeasurements(
	recordBuilder *array.RecordBuilder,
	measurements []*data.Measurement,
	epoch int64,
) error {
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
		maturityBuilder.Append(measurement.Maturity())
		snrBuilder.Append(measurement.SNR())
		snrDefinedBuilder.Append(true)
		tickBuilder.Append(measurement.Tick)

		metricsBuilder.Append(true)

		for entry := range measurement.Read() {
			if entry.Err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO,
					"[iceberg] measurement metric read failed during write",
					entry.Err,
				))
			}

			if entry.Metric == nil {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					"[iceberg] measurement metric missing during write",
					nil,
				))
			}

			metricsKey.Append(entry.Key)
			metricsVal.Append(entry.Metric.Raw)
		}

		metadataBuilder.Append(true)

		for _, key := range []string{"type", "order_id", "side", "event", "checksum", "ord_type", "trade_id", "status", "peer"} {
			if value := measurement.Meta(key); value != "" {
				metadataKey.Append(key)
				metadataVal.Append(value)
			}
		}

		provenanceBuilder.Append(true)

		if measurement.Error() != nil {
			provenanceKey.Append("symm:error")
			provenanceVal.Append(fmt.Sprintf("%v", measurement.Error()))
		}

		for entry := range measurement.Read() {
			if entry.Err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO,
					"[iceberg] measurement provenance read failed during write",
					entry.Err,
				))
			}

			if entry.Metric == nil {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					"[iceberg] measurement metric missing during provenance write",
					nil,
				))
			}

			if entry.Metric.Exact != nil {
				provenanceKey.Append("symm:exact:" + entry.Key)
				provenanceVal.Append(entry.Metric.Exact.String())
			}

			provenanceKey.Append("symm:standardized:" + entry.Key)
			provenanceVal.Append(strconv.FormatFloat(entry.Metric.Standardized, 'g', -1, 64))

			provenanceKey.Append("symm:normalized:" + entry.Key)
			provenanceVal.Append(strconv.FormatFloat(entry.Metric.Normalized, 'g', -1, 64))
		}
	}

	return nil
}

func ReadMeasurements(batch arrow.RecordBatch) ([]*data.Measurement, error) {
	totalRows := int(batch.NumRows())
	measurements := make([]*data.Measurement, 0, totalRows)

	cols := make(map[string]arrow.Array, batch.NumCols())

	for colIdx := range int(batch.NumCols()) {
		cols[batch.ColumnName(colIdx)] = batch.Column(colIdx)
	}

	epochCol, _ := cols["epoch"].(*array.Int64)
	seqIdxCol, _ := cols["seqIdx"].(*array.Int64)
	sourceCol, _ := cols["source"].(*array.String)
	labelCol, _ := cols["label"].(*array.String)
	atCol, _ := cols["at"].(*array.Timestamp)
	metricsCol, _ := cols["metrics"].(*array.Map)
	metadataCol, _ := cols["metadata"].(*array.Map)
	provenanceCol, _ := cols["provenance"].(*array.Map)
	tickCol, _ := cols["tick"].(*array.Int64)
	maturityCol, _ := cols["maturity"].(*array.Float64)
	snrCol, _ := cols["snr"].(*array.Float64)

	// SNR and Maturity are the confidence the Measurement's Metrics share,
	// which a replay cannot re-derive. A projection that selects neither
	// (Labels) reads no replayable tape; a batch carrying only one of them
	// is not a faithful one.
	confident := maturityCol != nil && snrCol != nil

	if !confident && (maturityCol != nil || snrCol != nil) {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"iceberg: measurement batch carries only one of maturity/snr",
			nil,
		))
	}

	rowMetrics := make(map[string]*metricHolder, 32)
	metricKeys := make([]string, 0, 32)
	holderPool := make([]metricHolder, 0, 32)

	for rowIdx := range totalRows {
		clear(rowMetrics)
		metricKeys = metricKeys[:0]
		holderPool = holderPool[:0]

		getHolder := func(key string) *metricHolder {
			if holder, exists := rowMetrics[key]; exists {
				return holder
			}

			holderPool = append(holderPool, metricHolder{})
			holder := &holderPool[len(holderPool)-1]
			rowMetrics[key] = holder
			metricKeys = append(metricKeys, key)
			return holder
		}

		var epoch int64

		if epochCol != nil && !epochCol.IsNull(rowIdx) {
			epoch = epochCol.Value(rowIdx)
		}

		var seqIdx int64

		if seqIdxCol != nil && !seqIdxCol.IsNull(rowIdx) {
			seqIdx = seqIdxCol.Value(rowIdx)
		}

		var tick int64

		if tickCol != nil && !tickCol.IsNull(rowIdx) {
			tick = tickCol.Value(rowIdx)
		}

		source := ""

		if sourceCol != nil && !sourceCol.IsNull(rowIdx) {
			source = sourceCol.Value(rowIdx)
		}

		label := ""

		if labelCol != nil && !labelCol.IsNull(rowIdx) {
			label = labelCol.Value(rowIdx)
		}

		var metadata []*data.StringEntry

		if metadataCol != nil && !metadataCol.IsNull(rowIdx) {
			keyArray := metadataCol.Keys().(*array.String)
			valArray := metadataCol.Items().(*array.String)
			offsets := metadataCol.Offsets()
			startOffset := int(offsets[rowIdx])
			endOffset := int(offsets[rowIdx+1])

			for itemIdx := startOffset; itemIdx < endOffset; itemIdx++ {
				metadata = append(metadata, &data.StringEntry{
					Key:   keyArray.Value(itemIdx),
					Value: valArray.Value(itemIdx),
				})
			}
		}

		measurement := data.NewMeasurement(
			epoch,
			label,
			source,
			seqIdx,
			tick,
			metadata...,
		)

		if confident && (maturityCol.IsNull(rowIdx) || snrCol.IsNull(rowIdx)) {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("iceberg: measurement row %d has no maturity/snr", rowIdx),
				nil,
			))
		}

		if confident {
			measurement.Restore(snrCol.Value(rowIdx), maturityCol.Value(rowIdx))
		}

		if atCol != nil && !atCol.IsNull(rowIdx) {
			measurement.At = time.UnixMicro(int64(atCol.Value(rowIdx))).UTC()
			measurement.From = measurement.At
			measurement.Timestamp = int64(atCol.Value(rowIdx)) * 1000
		}

		if metricsCol != nil && !metricsCol.IsNull(rowIdx) {
			keyArray := metricsCol.Keys().(*array.String)
			valArray := metricsCol.Items().(*array.Float64)
			offsets := metricsCol.Offsets()
			startOffset := int(offsets[rowIdx])
			endOffset := int(offsets[rowIdx+1])

			for itemIdx := startOffset; itemIdx < endOffset; itemIdx++ {
				metricKey := keyArray.Value(itemIdx)
				holder := getHolder(metricKey)
				holder.raw = valArray.Value(itemIdx)
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

				afterExact, isExact := strings.CutPrefix(key, "symm:exact:")

				if isExact {
					exact, err := decimal.NewFromString(value)

					if err != nil {
						return nil, errnie.Error(errnie.Err(
							errnie.Validation,
							"iceberg: invalid original decimal quantity",
							err,
						))
					}

					holder := getHolder(afterExact)
					holder.raw = exact.Float64()
					holder.exact = exact
				}

				afterStd, isStd := strings.CutPrefix(key, "symm:standardized:")

				if isStd {
					parsed, err := strconv.ParseFloat(value, 64)

					if err != nil {
						return nil, errnie.Error(errnie.Err(
							errnie.Validation,
							"iceberg: invalid standardized metric",
							err,
						))
					}

					holder := getHolder(afterStd)
					holder.standardized = parsed
					holder.hasStandardized = true
				}

				afterNorm, isNorm := strings.CutPrefix(key, "symm:normalized:")

				if isNorm {
					parsed, err := strconv.ParseFloat(value, 64)

					if err != nil {
						return nil, errnie.Error(errnie.Err(
							errnie.Validation,
							"iceberg: invalid normalized metric",
							err,
						))
					}

					holder := getHolder(afterNorm)
					holder.normalized = parsed
					holder.hasNormalized = true
				}
			}
		}

		metrics := make([]*data.Metric, 0, len(metricKeys))

		for _, key := range metricKeys {
			holder := rowMetrics[key]
			var metric *data.Metric

			if holder.exact != nil {
				metric = data.NewExactMetric(
					key,
					holder.exact,
					data.UnitDimensionless,
					data.TimescaleInstantaneous,
				)
			}

			if holder.exact == nil {
				metric = data.NewMetric(
					key,
					holder.raw,
					data.UnitDimensionless,
					data.TimescaleInstantaneous,
				)
			}

			if holder.hasStandardized {
				metric.Standardized = holder.standardized
			}

			if holder.hasNormalized {
				metric.Normalized = holder.normalized
			}

			metrics = append(metrics, metric)
		}

		measurement.Write(metrics...)

		// A replayable row (maturity/snr selected, so every column is) that
		// fails its own Write is corrupt tape, e.g. a metric stored with an
		// undefined raw by a producer that divided 0/0. Replaying it would
		// re-log the same failure on every pass and teach from a hole, so the
		// read halts here and names the row. A partial projection (Labels)
		// carries no replayable Measurement and is not validated as one.
		if err := measurement.Error(); confident && err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf(
					"iceberg: stored measurement row %d is invalid (epoch %d, source %s, label %s, tick %d)",
					rowIdx, epoch, source, label, tick,
				),
				err,
			))
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
	measurements []*data.Measurement,
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

	if err := fillMeasurements(recordBuilder, measurements, epoch); err != nil {
		return nil, err
	}

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
