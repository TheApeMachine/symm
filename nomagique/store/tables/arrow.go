package tables

import (
	"errors"
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

		if len(measurement.Provenance) == 0 && len(measurement.Metrics) == 0 && measurement.Err == nil {
			provenanceBuilder.AppendNull()
		}

		if len(measurement.Provenance) > 0 || len(measurement.Metrics) > 0 || measurement.Err != nil {
			provenanceBuilder.Append(true)

			for provKey, provVal := range measurement.Provenance {
				provenanceKey.Append(provKey)
				provenanceVal.Append(provVal)
			}
			if measurement.Err != nil {
				provenanceKey.Append("symm:error")
				provenanceVal.Append(measurement.Err.Error())
			}
			provenanceKey.Append("symm:estimated")
			provenanceVal.Append(strconv.FormatBool(measurement.Estimated))
			for key, metric := range measurement.Metrics {
				if metric.Exact != nil {
					provenanceKey.Append("symm:exact:" + key)
					provenanceVal.Append(metric.Exact.String())
				}
			}

		}
	}
}

func readMeasurements(batch arrow.RecordBatch) ([]*data.Measurement[float64], error) {
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
			if measurement.Metrics == nil {
				measurement.Metrics = make(map[string]data.Metric[float64])
			}

			keyArray := metricsCol.Keys().(*array.String)
			valArray := metricsCol.Items().(*array.Float64)
			start, end := metricsCol.ValueOffsets(rowIdx)
			startOffset, endOffset := int(start), int(end)

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
			if measurement.Metadata == nil {
				measurement.Metadata = make(map[string]string)
			}

			keyArray := metadataCol.Keys().(*array.String)
			valArray := metadataCol.Items().(*array.String)
			start, end := metadataCol.ValueOffsets(rowIdx)
			startOffset, endOffset := int(start), int(end)

			for itemIdx := startOffset; itemIdx < endOffset; itemIdx++ {
				measurement.Metadata[keyArray.Value(itemIdx)] = valArray.Value(itemIdx)
			}
		}

		if provenanceCol != nil && !provenanceCol.IsNull(rowIdx) {
			if measurement.Provenance == nil {
				measurement.Provenance = make(map[string]string)
			}

			keyArray := provenanceCol.Keys().(*array.String)
			valArray := provenanceCol.Items().(*array.String)
			start, end := provenanceCol.ValueOffsets(rowIdx)
			startOffset, endOffset := int(start), int(end)

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

				if strings.HasPrefix(key, "symm:exact:") {
					metricKey := strings.TrimPrefix(key, "symm:exact:")
					exact, err := decimal.NewFromString(value)

					if err != nil {
						return nil, errnie.Error(errnie.Err(errnie.Validation, "iceberg: invalid original decimal quantity", err))
					}

					metric := measurement.Metrics[metricKey]
					metric.Exact = exact
					measurement.Metrics[metricKey] = metric
					continue
				}

				measurement.Provenance[key] = value
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

type ExcursionRecord struct {
	Epoch              int64   `json:"epoch"`
	ID                 string  `json:"id"`
	Symbol             string  `json:"symbol"`
	Direction          string  `json:"direction"`
	ClearsFriction     bool    `json:"clearsFriction"`
	PrecursorStartTick int64   `json:"precursorStartTick"`
	AnchorTick         int64   `json:"anchorTick"`
	ExtremumTick       int64   `json:"extremumTick"`
	ExitTick           int64   `json:"exitTick"`
	PostEndTick        int64   `json:"postEndTick"`
	EntryPrice         float64 `json:"entryPrice"`
	ExtremumPrice      float64 `json:"extremumPrice"`
	ExitPrice          float64 `json:"exitPrice"`
	PositionSize       float64 `json:"positionSize"`
	Fee                float64 `json:"fee"`
	Profit             float64 `json:"profit"`
	ProfitFraction     float64 `json:"profitFraction"`
	GrossExcursion     float64 `json:"grossExcursion"`
	ObservationCount   int64   `json:"observationCount"`
	Status             string  `json:"status"`
}

func fillExcursions(
	recordBuilder *array.RecordBuilder,
	excursions []ExcursionRecord,
	epoch int64,
) {
	epochBuilder := recordBuilder.Field(0).(*array.Int64Builder)
	idBuilder := recordBuilder.Field(1).(*array.StringBuilder)
	symbolBuilder := recordBuilder.Field(2).(*array.StringBuilder)
	directionBuilder := recordBuilder.Field(3).(*array.StringBuilder)
	clearsFrictionBuilder := recordBuilder.Field(4).(*array.BooleanBuilder)
	precursorStartTickBuilder := recordBuilder.Field(5).(*array.Int64Builder)
	anchorTickBuilder := recordBuilder.Field(6).(*array.Int64Builder)
	extremumTickBuilder := recordBuilder.Field(7).(*array.Int64Builder)
	exitTickBuilder := recordBuilder.Field(8).(*array.Int64Builder)
	postEndTickBuilder := recordBuilder.Field(9).(*array.Int64Builder)
	entryPriceBuilder := recordBuilder.Field(10).(*array.Float64Builder)
	extremumPriceBuilder := recordBuilder.Field(11).(*array.Float64Builder)
	exitPriceBuilder := recordBuilder.Field(12).(*array.Float64Builder)
	positionSizeBuilder := recordBuilder.Field(13).(*array.Float64Builder)
	feeBuilder := recordBuilder.Field(14).(*array.Float64Builder)
	profitBuilder := recordBuilder.Field(15).(*array.Float64Builder)
	profitFractionBuilder := recordBuilder.Field(16).(*array.Float64Builder)
	grossExcursionBuilder := recordBuilder.Field(17).(*array.Float64Builder)
	observationCountBuilder := recordBuilder.Field(18).(*array.Int64Builder)
	statusBuilder := recordBuilder.Field(19).(*array.StringBuilder)

	for _, excursion := range excursions {
		recEpoch := excursion.Epoch

		if recEpoch <= 0 {
			recEpoch = epoch
		}

		epochBuilder.Append(recEpoch)
		idBuilder.Append(excursion.ID)
		symbolBuilder.Append(excursion.Symbol)
		directionBuilder.Append(excursion.Direction)
		clearsFrictionBuilder.Append(excursion.ClearsFriction)
		precursorStartTickBuilder.Append(excursion.PrecursorStartTick)
		anchorTickBuilder.Append(excursion.AnchorTick)
		extremumTickBuilder.Append(excursion.ExtremumTick)
		exitTickBuilder.Append(excursion.ExitTick)
		postEndTickBuilder.Append(excursion.PostEndTick)
		entryPriceBuilder.Append(excursion.EntryPrice)
		extremumPriceBuilder.Append(excursion.ExtremumPrice)
		exitPriceBuilder.Append(excursion.ExitPrice)
		positionSizeBuilder.Append(excursion.PositionSize)
		feeBuilder.Append(excursion.Fee)
		profitBuilder.Append(excursion.Profit)
		profitFractionBuilder.Append(excursion.ProfitFraction)
		grossExcursionBuilder.Append(excursion.GrossExcursion)
		observationCountBuilder.Append(excursion.ObservationCount)
		statusBuilder.Append(excursion.Status)
	}
}

func readExcursions(batch arrow.RecordBatch) []ExcursionRecord {
	totalRows := int(batch.NumRows())
	excursions := make([]ExcursionRecord, 0, totalRows)

	cols := make(map[string]arrow.Array, batch.NumCols())

	for colIdx := range int(batch.NumCols()) {
		cols[batch.ColumnName(colIdx)] = batch.Column(colIdx)
	}

	epochCol, _ := cols["epoch"].(*array.Int64)
	idCol, _ := cols["id"].(*array.String)
	symbolCol, _ := cols["symbol"].(*array.String)
	directionCol, _ := cols["direction"].(*array.String)
	clearsFrictionCol, _ := cols["clears_friction"].(*array.Boolean)
	precursorStartTickCol, _ := cols["precursor_start_tick"].(*array.Int64)
	anchorTickCol, _ := cols["anchor_tick"].(*array.Int64)
	extremumTickCol, _ := cols["extremum_tick"].(*array.Int64)
	exitTickCol, _ := cols["exit_tick"].(*array.Int64)
	postEndTickCol, _ := cols["post_end_tick"].(*array.Int64)
	entryPriceCol, _ := cols["entry_price"].(*array.Float64)
	extremumPriceCol, _ := cols["extremum_price"].(*array.Float64)
	exitPriceCol, _ := cols["exit_price"].(*array.Float64)
	positionSizeCol, _ := cols["position_size"].(*array.Float64)
	feeCol, _ := cols["fee"].(*array.Float64)
	profitCol, _ := cols["profit"].(*array.Float64)
	profitFractionCol, _ := cols["profit_fraction"].(*array.Float64)
	grossExcursionCol, _ := cols["gross_excursion"].(*array.Float64)
	observationCountCol, _ := cols["observation_count"].(*array.Int64)
	statusCol, _ := cols["status"].(*array.String)

	for rowIdx := range totalRows {
		excursion := ExcursionRecord{}

		if epochCol != nil && !epochCol.IsNull(rowIdx) {
			excursion.Epoch = epochCol.Value(rowIdx)
		}

		if idCol != nil && !idCol.IsNull(rowIdx) {
			excursion.ID = idCol.Value(rowIdx)
		}

		if symbolCol != nil && !symbolCol.IsNull(rowIdx) {
			excursion.Symbol = symbolCol.Value(rowIdx)
		}

		if directionCol != nil && !directionCol.IsNull(rowIdx) {
			excursion.Direction = directionCol.Value(rowIdx)
		}

		if clearsFrictionCol != nil && !clearsFrictionCol.IsNull(rowIdx) {
			excursion.ClearsFriction = clearsFrictionCol.Value(rowIdx)
		}

		if precursorStartTickCol != nil && !precursorStartTickCol.IsNull(rowIdx) {
			excursion.PrecursorStartTick = precursorStartTickCol.Value(rowIdx)
		}

		if anchorTickCol != nil && !anchorTickCol.IsNull(rowIdx) {
			excursion.AnchorTick = anchorTickCol.Value(rowIdx)
		}

		if extremumTickCol != nil && !extremumTickCol.IsNull(rowIdx) {
			excursion.ExtremumTick = extremumTickCol.Value(rowIdx)
		}

		if exitTickCol != nil && !exitTickCol.IsNull(rowIdx) {
			excursion.ExitTick = exitTickCol.Value(rowIdx)
		}

		if postEndTickCol != nil && !postEndTickCol.IsNull(rowIdx) {
			excursion.PostEndTick = postEndTickCol.Value(rowIdx)
		}

		if entryPriceCol != nil && !entryPriceCol.IsNull(rowIdx) {
			excursion.EntryPrice = entryPriceCol.Value(rowIdx)
		}

		if extremumPriceCol != nil && !extremumPriceCol.IsNull(rowIdx) {
			excursion.ExtremumPrice = extremumPriceCol.Value(rowIdx)
		}

		if exitPriceCol != nil && !exitPriceCol.IsNull(rowIdx) {
			excursion.ExitPrice = exitPriceCol.Value(rowIdx)
		}

		if positionSizeCol != nil && !positionSizeCol.IsNull(rowIdx) {
			excursion.PositionSize = positionSizeCol.Value(rowIdx)
		}

		if feeCol != nil && !feeCol.IsNull(rowIdx) {
			excursion.Fee = feeCol.Value(rowIdx)
		}

		if profitCol != nil && !profitCol.IsNull(rowIdx) {
			excursion.Profit = profitCol.Value(rowIdx)
		}

		if profitFractionCol != nil && !profitFractionCol.IsNull(rowIdx) {
			excursion.ProfitFraction = profitFractionCol.Value(rowIdx)
		}

		if grossExcursionCol != nil && !grossExcursionCol.IsNull(rowIdx) {
			excursion.GrossExcursion = grossExcursionCol.Value(rowIdx)
		}

		if observationCountCol != nil && !observationCountCol.IsNull(rowIdx) {
			excursion.ObservationCount = observationCountCol.Value(rowIdx)
		}

		if statusCol != nil && !statusCol.IsNull(rowIdx) {
			excursion.Status = statusCol.Value(rowIdx)
		}

		excursions = append(excursions, excursion)
	}

	return excursions
}

func excursionRecords(
	schema *iceberg.Schema,
	excursions []ExcursionRecord,
	epoch int64,
) (array.RecordReader, error) {
	converted, err := arrowSchemaFor(schema)

	if err != nil {
		return nil, err
	}

	recordBuilder := array.NewRecordBuilder(memory.DefaultAllocator, converted)
	defer recordBuilder.Release()

	recordBuilder.Reserve(len(excursions))
	fillExcursions(recordBuilder, excursions, epoch)
	batch := recordBuilder.NewRecord()
	defer batch.Release()

	reader, err := array.NewRecordReader(converted, []arrow.RecordBatch{batch})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"[iceberg] failed to build excursion record reader",
			err,
		))
	}

	return reader, nil
}
