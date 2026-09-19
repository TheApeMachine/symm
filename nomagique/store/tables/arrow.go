package tables

import (
	"fmt"
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

/*
BuildRecordBatch dynamically converts an arbitrary slice of map[string]any records
into an Apache Arrow RecordBatch matching the provided Iceberg schema.
No domain structs, pure dynamic Arrow generation.
*/
func BuildRecordBatch(schema *iceberg.Schema, records []map[string]any) (arrow.RecordBatch, error) {
	arrowSchema, err := arrowSchemaFor(schema)
	if err != nil {
		return nil, err
	}

	builder := array.NewRecordBuilder(memory.DefaultAllocator, arrowSchema)
	defer builder.Release()

	builder.Reserve(len(records))

	for _, rec := range records {
		for i, field := range arrowSchema.Fields() {
			val := rec[field.Name]
			appendFieldValue(builder.Field(i), field.Type, val)
		}
	}

	batch := builder.NewRecord()
	return batch, nil
}

func appendFieldValue(builder array.Builder, arrowType arrow.DataType, val any) {
	if val == nil {
		builder.AppendNull()
		return
	}

	switch b := builder.(type) {
	case *array.Int64Builder:
		switch v := val.(type) {
		case int64:
			b.Append(v)
		case int:
			b.Append(int64(v))
		case float64:
			b.Append(int64(v))
		default:
			builder.AppendNull()
		}
	case *array.Float64Builder:
		switch v := val.(type) {
		case float64:
			b.Append(v)
		case float32:
			b.Append(float64(v))
		case int:
			b.Append(float64(v))
		case int64:
			b.Append(float64(v))
		default:
			builder.AppendNull()
		}
	case *array.StringBuilder:
		switch v := val.(type) {
		case string:
			b.Append(v)
		default:
			b.Append(fmt.Sprint(v))
		}
	case *array.BooleanBuilder:
		switch v := val.(type) {
		case bool:
			b.Append(v)
		default:
			builder.AppendNull()
		}
	case *array.TimestampBuilder:
		switch v := val.(type) {
		case time.Time:
			b.Append(arrow.Timestamp(v.UTC().UnixMicro()))
		case int64:
			b.Append(arrow.Timestamp(v))
		default:
			builder.AppendNull()
		}
	case *array.BinaryBuilder:
		switch v := val.(type) {
		case []byte:
			b.Append(v)
		default:
			builder.AppendNull()
		}
	case *array.MapBuilder:
		if val == nil {
			b.AppendNull()
			return
		}
		keyBuilder, okKey := b.KeyBuilder().(*array.StringBuilder)
		valFloatBuilder, okValFloat := b.ItemBuilder().(*array.Float64Builder)
		valStrBuilder, okValStr := b.ItemBuilder().(*array.StringBuilder)

		switch mv := val.(type) {
		case map[string]float64:
			if len(mv) == 0 {
				b.AppendNull()
				return
			}
			b.Append(true)
			if okKey && okValFloat {
				for k, v := range mv {
					keyBuilder.Append(k)
					valFloatBuilder.Append(v)
				}
			}
		case map[string]string:
			if len(mv) == 0 {
				b.AppendNull()
				return
			}
			b.Append(true)
			if okKey && okValStr {
				for k, v := range mv {
					keyBuilder.Append(k)
					valStrBuilder.Append(v)
				}
			}
		case map[string]any:
			if len(mv) == 0 {
				b.AppendNull()
				return
			}
			b.Append(true)
			if okKey && okValFloat {
				for k, v := range mv {
					keyBuilder.Append(k)
					switch fv := v.(type) {
					case float64:
						valFloatBuilder.Append(fv)
					case int:
						valFloatBuilder.Append(float64(fv))
					case int64:
						valFloatBuilder.Append(float64(fv))
					default:
						valFloatBuilder.Append(0)
					}
				}
			}
			if okKey && okValStr {
				for k, v := range mv {
					keyBuilder.Append(k)
					valStrBuilder.Append(fmt.Sprint(v))
				}
			}
		default:
			b.AppendNull()
		}
	default:
		builder.AppendNull()
	}
}

/*
RecordBatchToMaps dynamically extracts rows from an Arrow RecordBatch into a slice of maps.
*/
func RecordBatchToMaps(batch arrow.RecordBatch) []map[string]any {
	totalRows := int(batch.NumRows())
	records := make([]map[string]any, totalRows)

	for rowIdx := range totalRows {
		rec := make(map[string]any, batch.NumCols())
		for colIdx := range int(batch.NumCols()) {
			colName := batch.ColumnName(colIdx)
			col := batch.Column(colIdx)
			if col.IsNull(rowIdx) {
				rec[colName] = nil
				continue
			}

			switch c := col.(type) {
			case *array.Int64:
				rec[colName] = c.Value(rowIdx)
			case *array.Float64:
				rec[colName] = c.Value(rowIdx)
			case *array.String:
				rec[colName] = c.Value(rowIdx)
			case *array.Boolean:
				rec[colName] = c.Value(rowIdx)
			case *array.Timestamp:
				rec[colName] = time.UnixMicro(int64(c.Value(rowIdx))).UTC()
			case *array.Binary:
				rec[colName] = c.Value(rowIdx)
			case *array.Map:
				start, end := c.ValueOffsets(rowIdx)
				m := make(map[string]any)
				if keyArr, ok := c.Keys().(*array.String); ok {
					if valFloat, okV := c.Items().(*array.Float64); okV {
						for idx := int(start); idx < int(end); idx++ {
							m[keyArr.Value(idx)] = valFloat.Value(idx)
						}
					}
					if valStr, okV := c.Items().(*array.String); okV {
						for idx := int(start); idx < int(end); idx++ {
							m[keyArr.Value(idx)] = valStr.Value(idx)
						}
					}
				}
				rec[colName] = m
			default:
				rec[colName] = nil
			}
		}
		records[rowIdx] = rec
	}

	return records
}

/*
RecordsToReader converts generic map records into an Arrow RecordReader for Iceberg appends.
*/
func RecordsToReader(schema *iceberg.Schema, records []map[string]any) (array.RecordReader, error) {
	converted, err := arrowSchemaFor(schema)
	if err != nil {
		return nil, err
	}

	batch, err := BuildRecordBatch(schema, records)
	if err != nil {
		return nil, err
	}
	defer batch.Release()

	reader, err := array.NewRecordReader(converted, []arrow.RecordBatch{batch})
	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"[iceberg] failed to build record reader",
			err,
		))
	}

	return reader, nil
}

func runRecords(schema *iceberg.Schema, runs []Run) (array.RecordReader, error) {
	maps := make([]map[string]any, len(runs))
	for i, r := range runs {
		maps[i] = map[string]any{
			"epoch":         r.Epoch,
			"started_at":    r.StartedAt,
			"code_commit":   r.CodeCommit,
			"build_id":      r.BuildID,
			"config_digest": r.ConfigDigest,
			"status":        r.Status,
		}
	}
	return RecordsToReader(schema, maps)
}

func readRuns(batch arrow.RecordBatch) []Run {
	maps := RecordBatchToMaps(batch)
	runs := make([]Run, len(maps))
	for i, m := range maps {
		var epoch int64
		if e, ok := m["epoch"].(int64); ok {
			epoch = e
		}
		var startedAt time.Time
		if s, ok := m["started_at"].(time.Time); ok {
			startedAt = s
		}
		var commit, buildID, digest, status string
		if s, ok := m["code_commit"].(string); ok {
			commit = s
		}
		if s, ok := m["build_id"].(string); ok {
			buildID = s
		}
		if s, ok := m["config_digest"].(string); ok {
			digest = s
		}
		if s, ok := m["status"].(string); ok {
			status = s
		}
		runs[i] = Run{
			Epoch:        epoch,
			StartedAt:    startedAt,
			CodeCommit:   commit,
			BuildID:      buildID,
			ConfigDigest: digest,
			Status:       status,
		}
	}
	return runs
}

func excursionRecords(schema *iceberg.Schema, excursions []ExcursionRecord, epoch int64) (array.RecordReader, error) {
	maps := make([]map[string]any, len(excursions))
	for i, e := range excursions {
		recEpoch := e.Epoch
		if recEpoch == 0 {
			recEpoch = epoch
		}
		maps[i] = map[string]any{
			"epoch":                recEpoch,
			"id":                   e.ID,
			"symbol":               e.Symbol,
			"direction":            e.Direction,
			"clears_friction":      e.ClearsFriction,
			"precursor_start_tick": e.PrecursorStartTick,
			"anchor_tick":          e.AnchorTick,
			"extremum_tick":        e.ExtremumTick,
			"exit_tick":            e.ExitTick,
			"post_end_tick":        e.PostEndTick,
			"entry_price":          e.EntryPrice,
			"extremum_price":       e.ExtremumPrice,
			"exit_price":           e.ExitPrice,
			"position_size":        e.PositionSize,
			"fee":                  e.Fee,
			"profit":               e.Profit,
			"profit_fraction":      e.ProfitFraction,
			"gross_excursion":      e.GrossExcursion,
			"observation_count":    e.ObservationCount,
			"status":               e.Status,
		}
	}
	return RecordsToReader(schema, maps)
}

func readExcursions(batch arrow.RecordBatch) []ExcursionRecord {
	maps := RecordBatchToMaps(batch)
	records := make([]ExcursionRecord, len(maps))
	for i, m := range maps {
		rec := ExcursionRecord{}
		if v, ok := m["epoch"].(int64); ok {
			rec.Epoch = v
		}
		if v, ok := m["id"].(string); ok {
			rec.ID = v
		}
		if v, ok := m["symbol"].(string); ok {
			rec.Symbol = v
		}
		if v, ok := m["direction"].(string); ok {
			rec.Direction = v
		}
		if v, ok := m["clears_friction"].(bool); ok {
			rec.ClearsFriction = v
		}
		if v, ok := m["precursor_start_tick"].(int64); ok {
			rec.PrecursorStartTick = v
		}
		if v, ok := m["anchor_tick"].(int64); ok {
			rec.AnchorTick = v
		}
		if v, ok := m["extremum_tick"].(int64); ok {
			rec.ExtremumTick = v
		}
		if v, ok := m["exit_tick"].(int64); ok {
			rec.ExitTick = v
		}
		if v, ok := m["post_end_tick"].(int64); ok {
			rec.PostEndTick = v
		}
		if v, ok := m["entry_price"].(float64); ok {
			rec.EntryPrice = v
		}
		if v, ok := m["extremum_price"].(float64); ok {
			rec.ExtremumPrice = v
		}
		if v, ok := m["exit_price"].(float64); ok {
			rec.ExitPrice = v
		}
		if v, ok := m["position_size"].(float64); ok {
			rec.PositionSize = v
		}
		if v, ok := m["fee"].(float64); ok {
			rec.Fee = v
		}
		if v, ok := m["profit"].(float64); ok {
			rec.Profit = v
		}
		if v, ok := m["profit_fraction"].(float64); ok {
			rec.ProfitFraction = v
		}
		if v, ok := m["gross_excursion"].(float64); ok {
			rec.GrossExcursion = v
		}
		if v, ok := m["observation_count"].(int64); ok {
			rec.ObservationCount = v
		}
		if v, ok := m["status"].(string); ok {
			rec.Status = v
		}
		records[i] = rec
	}
	return records
}

func measurementRecords(schema *iceberg.Schema, measurements []*data.Measurement[float64], epoch int64) (array.RecordReader, error) {
	maps := make([]map[string]any, len(measurements))
	for i, m := range measurements {
		metricMap := make(map[string]float64)
		for k, v := range m.Metrics {
			metricMap[k] = v.Raw
		}
		maps[i] = map[string]any{
			"epoch":       epoch,
			"tick":        m.SeqIdx,
			"source":      m.Source,
			"symbol":      m.Label,
			"venue_at":    m.At,
			"maturity":    m.Maturity,
			"snr":         m.SNR,
			"snr_defined": m.SNRDefined,
			"metrics":     metricMap,
			"metadata":    m.Metadata,
			"provenance":  m.Provenance,
		}
	}
	return RecordsToReader(schema, maps)
}

func readMeasurements(batch arrow.RecordBatch) ([]*data.Measurement[float64], error) {
	maps := RecordBatchToMaps(batch)
	measurements := make([]*data.Measurement[float64], len(maps))
	for i, m := range maps {
		meas := data.NewMeasurement[float64]("", nil)
		if v, ok := m["tick"].(int64); ok {
			meas.SeqIdx = v
		}
		if v, ok := m["source"].(string); ok {
			meas.Source = v
		}
		if v, ok := m["symbol"].(string); ok {
			meas.Label = v
		}
		if v, ok := m["venue_at"].(time.Time); ok {
			meas.At = v
		}
		if v, ok := m["maturity"].(float64); ok {
			meas.Maturity = v
		}
		if v, ok := m["snr"].(float64); ok {
			meas.SNR = v
		}
		if v, ok := m["snr_defined"].(bool); ok {
			meas.SNRDefined = v
		}
		switch met := m["metrics"].(type) {
		case map[string]float64:
			meas.Metrics = make(map[string]data.Metric[float64])
			for mk, mv := range met {
				meas.Metrics[mk] = data.Metric[float64]{
					Label: mk,
					Raw:   mv,
				}
			}
		case map[string]any:
			meas.Metrics = make(map[string]data.Metric[float64])
			for mk, mv := range met {
				if f, okF := mv.(float64); okF {
					meas.Metrics[mk] = data.Metric[float64]{
						Label: mk,
						Raw:   f,
					}
				}
			}
		}

		switch md := m["metadata"].(type) {
		case map[string]string:
			meas.Metadata = md
		case map[string]any:
			meas.Metadata = make(map[string]string)
			for mk, mv := range md {
				meas.Metadata[mk] = fmt.Sprint(mv)
			}
		}

		switch prov := m["provenance"].(type) {
		case map[string]string:
			meas.Provenance = prov
		case map[string]any:
			meas.Provenance = make(map[string]string)
			for mk, mv := range prov {
				meas.Provenance[mk] = fmt.Sprint(mv)
			}
		}
		measurements[i] = meas
	}
	return measurements, nil
}

