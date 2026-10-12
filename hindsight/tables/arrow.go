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
	measurements []*data.Measurement,
	epoch int64,
) error {
	for _, measurement := range measurements {
		if measurement == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"[iceberg] measurement is nil",
				nil,
			))
		}

		if err := measurement.Encode(recordBuilder, epoch); err != nil {
			return err
		}
	}

	return nil
}

func ReadMeasurements(batch arrow.RecordBatch) ([]*data.Measurement, error) {
	return data.DecodeBatch(batch)
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

func fillPositions(recordBuilder *array.RecordBuilder, positions []PositionRecord) {
	idBuilder := recordBuilder.Field(0).(*array.StringBuilder)
	symbolBuilder := recordBuilder.Field(1).(*array.StringBuilder)
	statusBuilder := recordBuilder.Field(2).(*array.StringBuilder)
	openedAtBuilder := recordBuilder.Field(3).(*array.TimestampBuilder)
	closedAtBuilder := recordBuilder.Field(4).(*array.TimestampBuilder)
	eventsBuilder := recordBuilder.Field(5).(*array.StringBuilder)
	venueCostBuilder := recordBuilder.Field(6).(*array.Float64Builder)
	venueProceedsBuilder := recordBuilder.Field(7).(*array.Float64Builder)
	venueFeesBuilder := recordBuilder.Field(8).(*array.Float64Builder)
	venueRealizedBuilder := recordBuilder.Field(9).(*array.Float64Builder)
	holdNsBuilder := recordBuilder.Field(10).(*array.Int64Builder)
	outcomeBuilder := recordBuilder.Field(11).(*array.StringBuilder)

	for _, pos := range positions {
		idBuilder.Append(pos.ID)
		symbolBuilder.Append(pos.Symbol)
		statusBuilder.Append(pos.Status)
		openedAtBuilder.Append(arrow.Timestamp(pos.OpenedAt.UTC().UnixMicro()))

		if pos.ClosedAt != nil {
			closedAtBuilder.Append(arrow.Timestamp(pos.ClosedAt.UTC().UnixMicro()))
		}

		if pos.ClosedAt == nil {
			closedAtBuilder.AppendNull()
		}

		eventsBuilder.Append(pos.Events)
		venueCostBuilder.Append(pos.VenueCost)
		venueProceedsBuilder.Append(pos.VenueProceeds)
		venueFeesBuilder.Append(pos.VenueFees)
		venueRealizedBuilder.Append(pos.VenueRealized)
		holdNsBuilder.Append(pos.HoldNs)

		if pos.Outcome != "" {
			outcomeBuilder.Append(pos.Outcome)
		}

		if pos.Outcome == "" {
			outcomeBuilder.AppendNull()
		}
	}
}

func readPositions(batch arrow.RecordBatch) []PositionRecord {
	totalRows := int(batch.NumRows())
	positions := make([]PositionRecord, 0, totalRows)

	cols := make(map[string]arrow.Array, batch.NumCols())

	for colIdx := range int(batch.NumCols()) {
		cols[batch.ColumnName(colIdx)] = batch.Column(colIdx)
	}

	idCol, _ := cols["id"].(*array.String)
	symbolCol, _ := cols["symbol"].(*array.String)
	statusCol, _ := cols["status"].(*array.String)
	openedAtCol, _ := cols["opened_at"].(*array.Timestamp)
	closedAtCol, _ := cols["closed_at"].(*array.Timestamp)
	eventsCol, _ := cols["events"].(*array.String)
	venueCostCol, _ := cols["venue_cost"].(*array.Float64)
	venueProceedsCol, _ := cols["venue_proceeds"].(*array.Float64)
	venueFeesCol, _ := cols["venue_fees"].(*array.Float64)
	venueRealizedCol, _ := cols["venue_realized"].(*array.Float64)
	holdNsCol, _ := cols["hold_ns"].(*array.Int64)
	outcomeCol, _ := cols["outcome"].(*array.String)

	for rowIdx := range totalRows {
		rec := PositionRecord{}

		if idCol != nil && !idCol.IsNull(rowIdx) {
			rec.ID = idCol.Value(rowIdx)
		}

		if symbolCol != nil && !symbolCol.IsNull(rowIdx) {
			rec.Symbol = symbolCol.Value(rowIdx)
		}

		if statusCol != nil && !statusCol.IsNull(rowIdx) {
			rec.Status = statusCol.Value(rowIdx)
		}

		if openedAtCol != nil && !openedAtCol.IsNull(rowIdx) {
			rec.OpenedAt = time.UnixMicro(int64(openedAtCol.Value(rowIdx))).UTC()
		}

		if closedAtCol != nil && !closedAtCol.IsNull(rowIdx) {
			timestampVal := time.UnixMicro(int64(closedAtCol.Value(rowIdx))).UTC()
			rec.ClosedAt = &timestampVal
		}

		if eventsCol != nil && !eventsCol.IsNull(rowIdx) {
			rec.Events = eventsCol.Value(rowIdx)
		}

		if venueCostCol != nil && !venueCostCol.IsNull(rowIdx) {
			rec.VenueCost = venueCostCol.Value(rowIdx)
		}

		if venueProceedsCol != nil && !venueProceedsCol.IsNull(rowIdx) {
			rec.VenueProceeds = venueProceedsCol.Value(rowIdx)
		}

		if venueFeesCol != nil && !venueFeesCol.IsNull(rowIdx) {
			rec.VenueFees = venueFeesCol.Value(rowIdx)
		}

		if venueRealizedCol != nil && !venueRealizedCol.IsNull(rowIdx) {
			rec.VenueRealized = venueRealizedCol.Value(rowIdx)
		}

		if holdNsCol != nil && !holdNsCol.IsNull(rowIdx) {
			rec.HoldNs = holdNsCol.Value(rowIdx)
		}

		if outcomeCol != nil && !outcomeCol.IsNull(rowIdx) {
			rec.Outcome = outcomeCol.Value(rowIdx)
		}

		positions = append(positions, rec)
	}

	return positions
}

func positionRecords(schema *iceberg.Schema, positions []PositionRecord) (array.RecordReader, error) {
	converted, err := arrowSchemaFor(schema)

	if err != nil {
		return nil, err
	}

	recordBuilder := array.NewRecordBuilder(memory.DefaultAllocator, converted)
	defer recordBuilder.Release()

	recordBuilder.Reserve(len(positions))
	fillPositions(recordBuilder, positions)
	batch := recordBuilder.NewRecordBatch()
	defer batch.Release()

	reader, err := array.NewRecordReader(converted, []arrow.RecordBatch{batch})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"[iceberg] failed to build position record reader",
			err,
		))
	}

	return reader, nil
}
