package tables

import (
	"time"

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
	tickBuilder := recordBuilder.Field(1).(*array.Int64Builder)
	sourceBuilder := recordBuilder.Field(2).(*array.StringBuilder)
	symbolBuilder := recordBuilder.Field(3).(*array.StringBuilder)
	venueAtBuilder := recordBuilder.Field(4).(*array.TimestampBuilder)
	maturityBuilder := recordBuilder.Field(5).(*array.Float64Builder)
	snrBuilder := recordBuilder.Field(6).(*array.Float64Builder)
	snrDefinedBuilder := recordBuilder.Field(7).(*array.BooleanBuilder)
	metricsBuilder := recordBuilder.Field(8).(*array.MapBuilder)
	metadataBuilder := recordBuilder.Field(9).(*array.MapBuilder)
	provenanceBuilder := recordBuilder.Field(10).(*array.MapBuilder)

	metricsKey := metricsBuilder.KeyBuilder().(*array.StringBuilder)
	metricsVal := metricsBuilder.ItemBuilder().(*array.Float64Builder)

	metadataKey := metadataBuilder.KeyBuilder().(*array.StringBuilder)
	metadataVal := metadataBuilder.ItemBuilder().(*array.StringBuilder)

	provenanceKey := provenanceBuilder.KeyBuilder().(*array.StringBuilder)
	provenanceVal := provenanceBuilder.ItemBuilder().(*array.StringBuilder)

	for _, measurement := range measurements {
		epochBuilder.Append(epoch)
		tickBuilder.Append(measurement.SeqIdx)
		sourceBuilder.Append(measurement.Source)
		symbolBuilder.Append(measurement.Label)

		if measurement.At.IsZero() {
			venueAtBuilder.Append(arrow.Timestamp(time.Now().UTC().UnixMicro()))
		}

		if !measurement.At.IsZero() {
			venueAtBuilder.Append(arrow.Timestamp(measurement.At.UTC().UnixMicro()))
		}

		maturityBuilder.Append(measurement.Maturity)

		if measurement.SNRDefined {
			snrBuilder.Append(measurement.SNR)
		}

		if !measurement.SNRDefined {
			snrBuilder.AppendNull()
		}

		snrDefinedBuilder.Append(measurement.SNRDefined)

		if len(measurement.Metrics) == 0 {
			metricsBuilder.AppendNull()
		}

		if len(measurement.Metrics) > 0 {
			metricsBuilder.Append(true)

			for metricKey, metricVal := range measurement.Metrics {
				metricsKey.Append(metricKey)
				metricsVal.Append(metricVal.Raw)
			}
		}

		if len(measurement.Metadata) == 0 {
			metadataBuilder.AppendNull()
		}

		if len(measurement.Metadata) > 0 {
			metadataBuilder.Append(true)

			for metaKey, metaVal := range measurement.Metadata {
				metadataKey.Append(metaKey)
				metadataVal.Append(metaVal)
			}
		}

		if len(measurement.Provenance) == 0 {
			provenanceBuilder.AppendNull()
		}

		if len(measurement.Provenance) > 0 {
			provenanceBuilder.Append(true)

			for provKey, provVal := range measurement.Provenance {
				provenanceKey.Append(provKey)
				provenanceVal.Append(provVal)
			}
		}
	}
}

func readMeasurements(batch arrow.RecordBatch) []*data.Measurement[float64] {
	totalRows := int(batch.NumRows())
	measurements := make([]*data.Measurement[float64], 0, totalRows)

	cols := make(map[string]arrow.Array, batch.NumCols())

	for colIdx := range int(batch.NumCols()) {
		cols[batch.ColumnName(colIdx)] = batch.Column(colIdx)
	}

	tickCol, _ := cols["tick"].(*array.Int64)
	sourceCol, _ := cols["source"].(*array.String)
	symbolCol, _ := cols["symbol"].(*array.String)
	venueAtCol, _ := cols["venue_at"].(*array.Timestamp)
	maturityCol, _ := cols["maturity"].(*array.Float64)
	snrCol, _ := cols["snr"].(*array.Float64)
	snrDefinedCol, _ := cols["snr_defined"].(*array.Boolean)
	metricsCol, _ := cols["metrics"].(*array.Map)
	metadataCol, _ := cols["metadata"].(*array.Map)
	provenanceCol, _ := cols["provenance"].(*array.Map)

	for rowIdx := range totalRows {
		source := ""

		if sourceCol != nil && !sourceCol.IsNull(rowIdx) {
			source = sourceCol.Value(rowIdx)
		}

		measurement := data.NewMeasurement[float64](source, nil)

		if tickCol != nil && !tickCol.IsNull(rowIdx) {
			measurement.SeqIdx = tickCol.Value(rowIdx)
		}

		if symbolCol != nil && !symbolCol.IsNull(rowIdx) {
			measurement.Label = symbolCol.Value(rowIdx)
		}

		if venueAtCol != nil && !venueAtCol.IsNull(rowIdx) {
			measurement.At = time.UnixMicro(int64(venueAtCol.Value(rowIdx))).UTC()
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
				measurement.Metrics[metricKey] = data.Metric[float64]{
					Label: metricKey,
					Raw:   metricVal,
				}
			}
		}

		if metadataCol != nil && !metadataCol.IsNull(rowIdx) {
			keyArray := metadataCol.Keys().(*array.String)
			valArray := metadataCol.Items().(*array.String)
			offsets := metadataCol.Offsets()
			startOffset := int(offsets[rowIdx])
			endOffset := int(offsets[rowIdx+1])

			for itemIdx := startOffset; itemIdx < endOffset; itemIdx++ {
				measurement.Metadata[keyArray.Value(itemIdx)] = valArray.Value(itemIdx)
			}
		}

		if provenanceCol != nil && !provenanceCol.IsNull(rowIdx) {
			keyArray := provenanceCol.Keys().(*array.String)
			valArray := provenanceCol.Items().(*array.String)
			offsets := provenanceCol.Offsets()
			startOffset := int(offsets[rowIdx])
			endOffset := int(offsets[rowIdx+1])

			for itemIdx := startOffset; itemIdx < endOffset; itemIdx++ {
				measurement.Provenance[keyArray.Value(itemIdx)] = valArray.Value(itemIdx)
			}
		}

		measurements = append(measurements, measurement)
	}

	return measurements
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
	converted, err := arrowSchemaFor(schema)

	if err != nil {
		return nil, err
	}

	recordBuilder := array.NewRecordBuilder(memory.DefaultAllocator, converted)
	defer recordBuilder.Release()

	recordBuilder.Reserve(len(measurements))
	fillMeasurements(recordBuilder, measurements, epoch)
	batch := recordBuilder.NewRecord()
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
	batch := recordBuilder.NewRecord()
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
