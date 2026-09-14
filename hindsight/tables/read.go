package tables

import (
	"bytes"
	"context"
	"iter"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/iceberg-go"
	icetable "github.com/apache/iceberg-go/table"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

/*
scan reads one table, optionally restricted to an epoch and tick range, and
yields its record batches. Batches are borrowed for the duration of each yield;
retained values must be copied before advancing the iterator.
*/
func (catalog *Catalog) scan(
	ctx context.Context, name string, fields []string, filters ...iceberg.BooleanExpression,
) (iter.Seq2[arrow.RecordBatch, error], error) {
	loaded, err := catalog.Load(ctx, name)

	if err != nil {
		return nil, err
	}

	predicate := iceberg.BooleanExpression(iceberg.AlwaysTrue{})

	for _, filter := range filters {
		predicate = iceberg.NewAnd(predicate, filter)
	}

	options := []icetable.ScanOption{icetable.WithRowFilter(predicate)}

	if len(fields) > 0 {
		options = append(options, icetable.WithSelectedFields(fields...))
	}
	tasks, err := loaded.Scan(options...).PlanFiles(ctx)

	if err != nil {
		return nil, errnie.Error(err)
	}
	batchSize := int64(loaded.Metadata().Properties().GetInt(
		icetable.ParquetBatchSizeKey, icetable.ParquetBatchSizeDefault,
	))

	if batchSize <= 0 {
		return nil, errnie.Error(errnie.Err(errnie.Validation,
			"[iceberg] Parquet read batch size must be positive", nil))
	}

	for _, task := range tasks {
		if len(fields) > 0 && !slices.Contains(fields, "payload") {
			break
		}
		bound, err := catalog.payloadBound(ctx, loaded, task)

		if err != nil {
			return nil, errnie.Error(err)
		}

		if bound > 0 {
			batchSize = min(batchSize, max(1, math.MaxInt32/bound))
			break
		}
	}

	metadata, err := icetable.MetadataBuilderFromBase(loaded.Metadata(), loaded.MetadataLocation())

	if err != nil {
		return nil, errnie.Error(err)
	}

	if err := metadata.SetProperties(iceberg.Properties{
		icetable.ParquetBatchSizeKey: strconv.FormatInt(batchSize, 10),
	}); err != nil {
		return nil, errnie.Error(err)
	}
	bounded, err := metadata.Build()

	if err != nil {
		return nil, errnie.Error(err)
	}
	reader := icetable.New(loaded.Identifier(), bounded, loaded.MetadataLocation(), loaded.FS, nil)
	_, batches, err := reader.Scan(options...).ReadTasks(ctx, tasks)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to scan "+name,
			err,
		))
	}

	return func(yield func(arrow.RecordBatch, error) bool) {
		for batch, err := range batches {
			more := yield(batch, err)

			if batch != nil {
				batch.Release()
			}

			if !more {
				return
			}
		}
	}, nil
}

func forEpoch(epoch int64) iceberg.BooleanExpression {
	return iceberg.EqualTo(iceberg.Reference("epoch"), epoch)
}

func afterTick(tick int64) iceberg.BooleanExpression {
	return iceberg.GreaterThan(iceberg.Reference("tick"), tick)
}

func colMap(batch arrow.RecordBatch) map[string]arrow.Array {
	cols := make(map[string]arrow.Array, batch.NumCols())

	for colIdx := range int(batch.NumCols()) {
		cols[batch.ColumnName(colIdx)] = batch.Column(colIdx)
	}

	return cols
}

func colNum(cols map[string]arrow.Array, name string, row int) int64 {
	col, ok := cols[name]

	if !ok || col.IsNull(row) {
		return 0
	}

	return col.(*array.Int64).Value(row)
}


func colStr(cols map[string]arrow.Array, name string, row int) string {
	col, ok := cols[name]

	if !ok || col.IsNull(row) {
		return ""
	}

	return strings.Clone(col.(*array.String).Value(row))
}

func colFlt(cols map[string]arrow.Array, name string, row int) float64 {
	col, ok := cols[name]

	if !ok || col.IsNull(row) {
		return 0
	}

	return col.(*array.Float64).Value(row)
}

func colTime(cols map[string]arrow.Array, name string, row int) time.Time {
	col, ok := cols[name]

	if !ok || col.IsNull(row) {
		return time.Time{}
	}

	return col.(*array.Timestamp).Value(row).ToTime(arrow.Microsecond).UTC()
}

func colBool(cols map[string]arrow.Array, name string, row int) bool {
	col, ok := cols[name]

	if !ok || col.IsNull(row) {
		return false
	}

	return col.(*array.Boolean).Value(row)
}

func str(column arrow.Array, row int) string {
	if column.IsNull(row) {
		return ""
	}

	return strings.Clone(column.(*array.String).Value(row))
}

func num(column arrow.Array, row int) int64 {
	if column.IsNull(row) {
		return 0
	}

	return column.(*array.Int64).Value(row)
}

func num32(column arrow.Array, row int) int32 {
	if column.IsNull(row) {
		return 0
	}

	return column.(*array.Int32).Value(row)
}

func flt(column arrow.Array, row int) float64 {
	if column.IsNull(row) {
		return 0
	}

	return column.(*array.Float64).Value(row)
}

func fltPtr(column arrow.Array, row int) *float64 {
	if column.IsNull(row) {
		return nil
	}

	val := column.(*array.Float64).Value(row)
	return &val
}

func boolean(column arrow.Array, row int) bool {
	if column.IsNull(row) {
		return false
	}

	return column.(*array.Boolean).Value(row)
}

func when(column arrow.Array, row int) (value arrow.Timestamp, ok bool) {
	if column.IsNull(row) {
		return 0, false
	}

	return column.(*array.Timestamp).Value(row), true
}

func timeVal(column arrow.Array, row int) time.Time {
	micros, ok := when(column, row)

	if !ok {
		return time.Time{}
	}

	return micros.ToTime(arrow.Microsecond).UTC()
}

func timePtr(column arrow.Array, row int) *time.Time {
	micros, ok := when(column, row)

	if !ok {
		return nil
	}

	t := micros.ToTime(arrow.Microsecond).UTC()
	return &t
}

func bin(column arrow.Array, row int) []byte {
	if column.IsNull(row) {
		return nil
	}

	return bytes.Clone(column.(*array.Binary).Value(row))
}

func mapStrFlt(column arrow.Array, row int) map[string]float64 {
	if column.IsNull(row) {
		return nil
	}

	mapArr := column.(*array.Map)
	start, end := mapArr.ValueOffsets(row)
	result := make(map[string]float64, end-start)
	keys := mapArr.Keys().(*array.String)
	items := mapArr.Items().(*array.Float64)

	for element := int(start); element < int(end); element++ {
		result[strings.Clone(keys.Value(element))] = items.Value(element)
	}

	return result
}

func dec(column arrow.Array, row int) *decimal.Decimal {
	if column.IsNull(row) {
		return nil
	}

	unscaled := column.(*array.Decimal128).Value(row).BigInt()
	digits := unscaled.String()
	sign := ""

	if strings.HasPrefix(digits, "-") {
		sign, digits = "-", digits[1:]
	}

	for int64(len(digits)) <= DecimalScale {
		digits = "0" + digits
	}

	point := int64(len(digits)) - DecimalScale
	value, err := decimal.NewFromString(sign + digits[:point] + "." + digits[point:])

	if err != nil {
		errnie.Error(errnie.Err(errnie.Validation, "[iceberg] decode decimal", err))

		return nil
	}

	return value
}

// Measurements scans canonical measurements strictly after afterTick for a given epoch.
func (c *Catalog) Measurements(ctx context.Context, epoch int64, afterTickSeq int64) ([]MeasurementRow, error) {
	return c.MeasurementsScan(ctx, epoch, afterTick(afterTickSeq), 0)
}

// MeasurementsScan scans measurements matching filter with optional limit and selected fields.
func (c *Catalog) MeasurementsScan(
	ctx context.Context, epoch int64, filter iceberg.BooleanExpression, limit int, fields ...string,
) ([]MeasurementRow, error) {
	batches, err := c.scan(ctx, Measurements, fields, forEpoch(epoch), filter)

	if err != nil {
		return nil, err
	}

	rows := []MeasurementRow{}

	for batch, err := range batches {
		if err != nil {
			return nil, errnie.Error(errnie.Err(errnie.BadGateway, "[iceberg] measurements batch", err))
		}

		cols := colMap(batch)

		for index := range int(batch.NumRows()) {
			var m map[string]float64
			var md map[string]float64
			var p []byte

			if col, ok := cols["metrics"]; ok {
				m = mapStrFlt(col, index)
			}

			if col, ok := cols["metadata"]; ok {
				md = mapStrFlt(col, index)
			}

			if col, ok := cols["payload"]; ok {
				p = bin(col, index)
			}

			rows = append(rows, MeasurementRow{
				Epoch:      colNum(cols, "epoch", index),
				Tick:       colNum(cols, "tick", index),
				Source:     colStr(cols, "source", index),
				Symbol:     colStr(cols, "symbol", index),
				VenueAt:    colTime(cols, "venue_at", index),
				ObservedAt: colTime(cols, "observed_at", index),
				Maturity:   colFlt(cols, "maturity", index),
				SNR:        colFlt(cols, "snr", index),
				SNRDefined: colBool(cols, "snr_defined", index),
				Metrics:    m,
				Metadata:   md,
				Payload:    p,
			})

			if limit > 0 && len(rows) >= limit {
				return rows, nil
			}
		}
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Tick < rows[j].Tick })

	return rows, nil
}

// EachMeasurement streams canonical measurements strictly after afterTick for a given epoch.
func (c *Catalog) EachMeasurement(
	ctx context.Context, epoch int64, afterTickSeq int64, fn func(MeasurementRow) error,
) error {
	batches, err := c.scan(ctx, Measurements, nil, forEpoch(epoch), afterTick(afterTickSeq))

	if err != nil {
		return err
	}

	for batch, err := range batches {
		if err != nil {
			return errnie.Error(errnie.Err(errnie.BadGateway, "[iceberg] measurements batch", err))
		}

		cols := colMap(batch)

		for index := range int(batch.NumRows()) {
			var m map[string]float64
			var md map[string]float64
			var p []byte

			if col, ok := cols["metrics"]; ok {
				m = mapStrFlt(col, index)
			}

			if col, ok := cols["metadata"]; ok {
				md = mapStrFlt(col, index)
			}

			if col, ok := cols["payload"]; ok {
				p = bin(col, index)
			}

			row := MeasurementRow{
				Epoch:      colNum(cols, "epoch", index),
				Tick:       colNum(cols, "tick", index),
				Source:     colStr(cols, "source", index),
				Symbol:     colStr(cols, "symbol", index),
				VenueAt:    colTime(cols, "venue_at", index),
				ObservedAt: colTime(cols, "observed_at", index),
				Maturity:   colFlt(cols, "maturity", index),
				SNR:        colFlt(cols, "snr", index),
				SNRDefined: colBool(cols, "snr_defined", index),
				Metrics:    m,
				Metadata:   md,
				Payload:    p,
			}

			if err := fn(row); err != nil {
				return err
			}
		}
	}

	return nil
}

// SpotLevel3 scans book touch updates strictly after afterTick for a given epoch.
func (c *Catalog) SpotLevel3(ctx context.Context, epoch int64, afterTickSeq int64) ([]SpotLevel3Row, error) {
	return c.SpotLevel3Scan(ctx, epoch, afterTick(afterTickSeq), 0)
}

// SpotLevel3Scan scans book touch updates matching filter with optional limit and selected fields.
func (c *Catalog) SpotLevel3Scan(
	ctx context.Context, epoch int64, filter iceberg.BooleanExpression, limit int, fields ...string,
) ([]SpotLevel3Row, error) {
	batches, err := c.scan(ctx, SpotLevel3, fields, forEpoch(epoch), filter)

	if err != nil {
		return nil, err
	}

	rows := []SpotLevel3Row{}

	for batch, err := range batches {
		if err != nil {
			return nil, errnie.Error(errnie.Err(errnie.BadGateway, "[iceberg] spot_level3 batch", err))
		}

		cols := colMap(batch)

		for index := range int(batch.NumRows()) {
			rows = append(rows, SpotLevel3Row{
				Epoch:      colNum(cols, "epoch", index),
				Tick:       colNum(cols, "tick", index),
				Symbol:     colStr(cols, "symbol", index),
				VenueAt:    colTime(cols, "venue_at", index),
				ReceivedAt: colTime(cols, "received_at", index),
				Side:       colStr(cols, "side", index),
				Event:      colStr(cols, "event", index),
				OrderID:    colStr(cols, "order_id", index),
				LimitPrice: colFlt(cols, "limit_price", index),
				OrderQty:   colFlt(cols, "order_qty", index),
				Checksum:   colNum(cols, "checksum", index),
			})

			if limit > 0 && len(rows) >= limit {
				return rows, nil
			}
		}
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Tick < rows[j].Tick })

	return rows, nil
}

// SpotTicker scans spot ticker updates strictly after afterTick for a given epoch.
func (c *Catalog) SpotTicker(ctx context.Context, epoch int64, afterTickSeq int64) ([]SpotTickerRow, error) {
	return c.SpotTickerScan(ctx, epoch, afterTick(afterTickSeq), 0)
}

// SpotTickerScan scans spot ticker updates matching filter with optional limit and selected fields.
func (c *Catalog) SpotTickerScan(
	ctx context.Context, epoch int64, filter iceberg.BooleanExpression, limit int, fields ...string,
) ([]SpotTickerRow, error) {
	batches, err := c.scan(ctx, SpotTicker, fields, forEpoch(epoch), filter)

	if err != nil {
		return nil, err
	}

	rows := []SpotTickerRow{}

	for batch, err := range batches {
		if err != nil {
			return nil, errnie.Error(errnie.Err(errnie.BadGateway, "[iceberg] spot_ticker batch", err))
		}

		cols := colMap(batch)

		for index := range int(batch.NumRows()) {
			rows = append(rows, SpotTickerRow{
				Epoch:      colNum(cols, "epoch", index),
				Tick:       colNum(cols, "tick", index),
				Symbol:     colStr(cols, "symbol", index),
				VenueAt:    colTime(cols, "venue_at", index),
				ReceivedAt: colTime(cols, "received_at", index),
				Bid:        colFlt(cols, "bid", index),
				BidQty:     colFlt(cols, "bid_qty", index),
				Ask:        colFlt(cols, "ask", index),
				AskQty:     colFlt(cols, "ask_qty", index),
				Last:       colFlt(cols, "last", index),
				Volume:     colFlt(cols, "volume", index),
				VWAP:       colFlt(cols, "vwap", index),
				Low:        colFlt(cols, "low", index),
				High:       colFlt(cols, "high", index),
				Change:     colFlt(cols, "change", index),
				ChangePct:  colFlt(cols, "change_pct", index),
			})

			if limit > 0 && len(rows) >= limit {
				return rows, nil
			}
		}
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Tick < rows[j].Tick })

	return rows, nil
}

// SpotTrade scans spot trades strictly after afterTick for a given epoch.
func (c *Catalog) SpotTrade(ctx context.Context, epoch int64, afterTickSeq int64) ([]SpotTradeRow, error) {
	return c.SpotTradeScan(ctx, epoch, afterTick(afterTickSeq), 0)
}

// SpotTradeScan scans spot trades matching filter with optional limit and selected fields.
func (c *Catalog) SpotTradeScan(
	ctx context.Context, epoch int64, filter iceberg.BooleanExpression, limit int, fields ...string,
) ([]SpotTradeRow, error) {
	batches, err := c.scan(ctx, SpotTrade, fields, forEpoch(epoch), filter)

	if err != nil {
		return nil, err
	}

	rows := []SpotTradeRow{}

	for batch, err := range batches {
		if err != nil {
			return nil, errnie.Error(errnie.Err(errnie.BadGateway, "[iceberg] spot_trade batch", err))
		}

		cols := colMap(batch)

		for index := range int(batch.NumRows()) {
			rows = append(rows, SpotTradeRow{
				Epoch:      colNum(cols, "epoch", index),
				Tick:       colNum(cols, "tick", index),
				Symbol:     colStr(cols, "symbol", index),
				VenueAt:    colTime(cols, "venue_at", index),
				ReceivedAt: colTime(cols, "received_at", index),
				Price:      colFlt(cols, "price", index),
				Qty:        colFlt(cols, "qty", index),
				Side:       colStr(cols, "side", index),
				OrdType:    colStr(cols, "ord_type", index),
				TradeID:    colNum(cols, "trade_id", index),
			})

			if limit > 0 && len(rows) >= limit {
				return rows, nil
			}
		}
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Tick < rows[j].Tick })

	return rows, nil
}

// FuturesTicker scans futures ticker updates strictly after afterTick for a given epoch.
func (c *Catalog) FuturesTicker(ctx context.Context, epoch int64, afterTickSeq int64) ([]FuturesTickerRow, error) {
	batches, err := c.scan(ctx, FuturesTicker, nil, forEpoch(epoch), afterTick(afterTickSeq))

	if err != nil {
		return nil, err
	}

	rows := []FuturesTickerRow{}

	for batch, err := range batches {
		if err != nil {
			return nil, errnie.Error(errnie.Err(errnie.BadGateway, "[iceberg] futures_ticker batch", err))
		}

		for index := range int(batch.NumRows()) {
			rows = append(rows, FuturesTickerRow{
				Epoch:        num(batch.Column(0), index),
				Tick:         num(batch.Column(1), index),
				Symbol:       str(batch.Column(2), index),
				VenueAt:      timeVal(batch.Column(3), index),
				ReceivedAt:   timeVal(batch.Column(4), index),
				Bid:          flt(batch.Column(5), index),
				BidQty:       flt(batch.Column(6), index),
				Ask:          flt(batch.Column(7), index),
				AskQty:       flt(batch.Column(8), index),
				Last:         flt(batch.Column(9), index),
				Volume:       flt(batch.Column(10), index),
				VWAP:         flt(batch.Column(11), index),
				Low:          flt(batch.Column(12), index),
				High:         flt(batch.Column(13), index),
				Change:       flt(batch.Column(14), index),
				ChangePct:    flt(batch.Column(15), index),
				MarkPrice:    flt(batch.Column(16), index),
				IndexPrice:   flt(batch.Column(17), index),
				OpenInterest: flt(batch.Column(18), index),
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Tick < rows[j].Tick })

	return rows, nil
}

// FuturesTrade scans futures trades strictly after afterTick for a given epoch.
func (c *Catalog) FuturesTrade(ctx context.Context, epoch int64, afterTickSeq int64) ([]FuturesTradeRow, error) {
	batches, err := c.scan(ctx, FuturesTrade, nil, forEpoch(epoch), afterTick(afterTickSeq))

	if err != nil {
		return nil, err
	}

	rows := []FuturesTradeRow{}

	for batch, err := range batches {
		if err != nil {
			return nil, errnie.Error(errnie.Err(errnie.BadGateway, "[iceberg] futures_trade batch", err))
		}

		for index := range int(batch.NumRows()) {
			rows = append(rows, FuturesTradeRow{
				Epoch:      num(batch.Column(0), index),
				Tick:       num(batch.Column(1), index),
				Symbol:     str(batch.Column(2), index),
				VenueAt:    timeVal(batch.Column(3), index),
				ReceivedAt: timeVal(batch.Column(4), index),
				Price:      flt(batch.Column(5), index),
				Qty:        flt(batch.Column(6), index),
				Side:       str(batch.Column(7), index),
				OrdType:    str(batch.Column(8), index),
				TradeID:    num(batch.Column(9), index),
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Tick < rows[j].Tick })

	return rows, nil
}

// Executions scans executions updates strictly after afterTick for a given epoch.
func (c *Catalog) Executions(ctx context.Context, epoch int64, afterTickSeq int64) ([]ExecutionRow, error) {
	batches, err := c.scan(ctx, Executions, nil, forEpoch(epoch), afterTick(afterTickSeq))

	if err != nil {
		return nil, err
	}

	rows := []ExecutionRow{}

	for batch, err := range batches {
		if err != nil {
			return nil, errnie.Error(errnie.Err(errnie.BadGateway, "[iceberg] executions batch", err))
		}

		for index := range int(batch.NumRows()) {
			rows = append(rows, ExecutionRow{
				Epoch:        num(batch.Column(0), index),
				Tick:         num(batch.Column(1), index),
				Symbol:       str(batch.Column(2), index),
				VenueAt:      timeVal(batch.Column(3), index),
				ReceivedAt:   timeVal(batch.Column(4), index),
				OrderID:      str(batch.Column(5), index),
				OrderUserRef: num(batch.Column(6), index),
				ExecID:       str(batch.Column(7), index),
				ExecType:     str(batch.Column(8), index),
				TradeID:      num(batch.Column(9), index),
				Side:         str(batch.Column(10), index),
				LastQty:      dec(batch.Column(11), index),
				LastPrice:    dec(batch.Column(12), index),
				LiquidityInd: str(batch.Column(13), index),
				Cost:         dec(batch.Column(14), index),
				OrderType:    str(batch.Column(15), index),
				OrderStatus:  str(batch.Column(16), index),
				CumQty:       dec(batch.Column(17), index),
				CumCost:      dec(batch.Column(18), index),
				AvgPrice:     dec(batch.Column(19), index),
				FeeUsdEquiv:  dec(batch.Column(20), index),
				Fees:         str(batch.Column(21), index),
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Tick < rows[j].Tick })

	return rows, nil
}

// Models scans model snapshots strictly after afterTick for a given epoch.
func (c *Catalog) Models(ctx context.Context, epoch int64, afterTickSeq int64) ([]ModelRow, error) {
	batches, err := c.scan(ctx, Models, nil, forEpoch(epoch), afterTick(afterTickSeq))

	if err != nil {
		return nil, err
	}

	rows := []ModelRow{}

	for batch, err := range batches {
		if err != nil {
			return nil, errnie.Error(errnie.Err(errnie.BadGateway, "[iceberg] models batch", err))
		}

		for index := range int(batch.NumRows()) {
			rows = append(rows, ModelRow{
				Epoch:      num(batch.Column(0), index),
				Tick:       num(batch.Column(1), index),
				AgentID:    num32(batch.Column(2), index),
				StepCount:  num(batch.Column(3), index),
				NodesCount: num(batch.Column(4), index),
				Payload:    bin(batch.Column(5), index),
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Tick < rows[j].Tick })

	return rows, nil
}

// Grids scans perception grid snapshots strictly after afterTick for a given epoch.
func (c *Catalog) Grids(ctx context.Context, epoch int64, afterTickSeq int64) ([]GridRow, error) {
	batches, err := c.scan(ctx, Grids, nil, forEpoch(epoch), afterTick(afterTickSeq))

	if err != nil {
		return nil, err
	}

	rows := []GridRow{}

	for batch, err := range batches {
		if err != nil {
			return nil, errnie.Error(errnie.Err(errnie.BadGateway, "[iceberg] grids batch", err))
		}

		for index := range int(batch.NumRows()) {
			rows = append(rows, GridRow{
				Epoch:        num(batch.Column(0), index),
				Tick:         num(batch.Column(1), index),
				AgentID:      num32(batch.Column(2), index),
				ContextLabel: str(batch.Column(3), index),
				Payload:      bin(batch.Column(4), index),
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Tick < rows[j].Tick })

	return rows, nil
}

// Positions scans position records strictly after afterTick for a given epoch.
func (c *Catalog) Positions(ctx context.Context, epoch int64, afterTickSeq int64) ([]PositionRow, error) {
	batches, err := c.scan(ctx, Positions, nil, forEpoch(epoch), afterTick(afterTickSeq))

	if err != nil {
		return nil, err
	}

	rows := []PositionRow{}

	for batch, err := range batches {
		if err != nil {
			return nil, errnie.Error(errnie.Err(errnie.BadGateway, "[iceberg] positions batch", err))
		}

		for index := range int(batch.NumRows()) {
			rows = append(rows, PositionRow{
				Epoch:       num(batch.Column(0), index),
				Tick:        num(batch.Column(1), index),
				Symbol:      str(batch.Column(2), index),
				Status:      str(batch.Column(3), index),
				Qty:         dec(batch.Column(4), index),
				Basis:       dec(batch.Column(5), index),
				EntryPrice:  dec(batch.Column(6), index),
				EntryFee:    dec(batch.Column(7), index),
				ExitPrice:   dec(batch.Column(8), index),
				ExitFee:     dec(batch.Column(9), index),
				Mark:        dec(batch.Column(10), index),
				PnL:         dec(batch.Column(11), index),
				RealizedPnL: dec(batch.Column(12), index),
				EntryAt:     timePtr(batch.Column(13), index),
				ExitAt:      timePtr(batch.Column(14), index),
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Tick < rows[j].Tick })

	return rows, nil
}

// Decisions scans agent action decisions strictly after afterTick for a given epoch.
func (c *Catalog) Decisions(ctx context.Context, epoch int64, afterTickSeq int64) ([]OutcomeRow, error) {
	return c.outcomes(ctx, Decisions, epoch, afterTickSeq)
}

// Outcomes scans graded outcomes strictly after afterTick for a given epoch.
func (c *Catalog) Outcomes(ctx context.Context, epoch int64, afterTickSeq int64) ([]OutcomeRow, error) {
	return c.outcomes(ctx, Outcomes, epoch, afterTickSeq)
}

func (c *Catalog) outcomes(ctx context.Context, tableName string, epoch int64, afterTickSeq int64) ([]OutcomeRow, error) {
	batches, err := c.scan(ctx, tableName, nil, forEpoch(epoch), afterTick(afterTickSeq))

	if err != nil {
		return nil, err
	}

	rows := []OutcomeRow{}

	for batch, err := range batches {
		if err != nil {
			return nil, errnie.Error(errnie.Err(errnie.BadGateway, "[iceberg] "+tableName+" batch", err))
		}

		for index := range int(batch.NumRows()) {
			rows = append(rows, OutcomeRow{
				Epoch:        num(batch.Column(0), index),
				Tick:         num(batch.Column(1), index),
				DecisionID:   num(batch.Column(2), index),
				Symbol:       str(batch.Column(3), index),
				At:           timeVal(batch.Column(4), index),
				ActionKind:   str(batch.Column(5), index),
				ActionPower:  num32(batch.Column(6), index),
				ActionReduce: boolean(batch.Column(7), index),
				Authority:    flt(batch.Column(8), index),
				Outcome:      fltPtr(batch.Column(9), index),
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Tick < rows[j].Tick })

	return rows, nil
}
