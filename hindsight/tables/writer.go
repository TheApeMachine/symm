package tables

import (
	"context"
	"runtime"
	"sync/atomic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
)

const (
	tickerBatchThreshold      = 2000
	tradeBatchThreshold       = 2000
	level3BatchThreshold      = 20000
	measurementBatchThreshold = 2000
	writerMask                = 1 << 30
)

/*
Writer buffers incoming measurements per canonical table family and commits them
to Iceberg using per-family volume thresholds. This prevents high-volume Level 3
streams from forcing frequent micro-file commits across quiet ticker and trade families.
*/
type Writer struct {
	catalog *Catalog
	epoch   int64

	gate         atomic.Int64
	spotTicker     []*data.Measurement[float64]
	spotTrade      []*data.Measurement[float64]
	spotLevel3     []*data.Measurement[float64]
	futuresTicker  []*data.Measurement[float64]
	futuresTrade   []*data.Measurement[float64]
	measurements   []*data.Measurement[float64]
	excursions   []ExcursionRecord
}

func (writer *Writer) lock() {
	for {
		value := writer.gate.Load()

		if value == 0 && writer.gate.CompareAndSwap(0, -writerMask) {
			return
		}

		runtime.Gosched()
	}
}

func (writer *Writer) unlock() {
	writer.gate.Add(writerMask)
}

/*
NewWriter constructs a Writer writing into the given catalog for a stable run epoch.
*/
func NewWriter(catalog *Catalog, epoch int64) *Writer {
	return &Writer{
		catalog: catalog,
		epoch:   epoch,
	}
}

/*
Add routes an incoming measurement to its canonical table family buffer.
*/
func (writer *Writer) Add(channel string, measurement *data.Measurement[float64]) {
	if measurement == nil {
		return
	}

	writer.lock()
	defer writer.unlock()

	if channel == "ticker" {
		writer.spotTicker = append(writer.spotTicker, measurement)

		return
	}

	if channel == "trade" {
		writer.spotTrade = append(writer.spotTrade, measurement)

		return
	}

	if channel == "level3" {
		writer.spotLevel3 = append(writer.spotLevel3, measurement)

		return
	}

	if channel == "futures_ticker" {
		writer.futuresTicker = append(writer.futuresTicker, measurement)

		return
	}

	if channel == "futures_trade" {
		writer.futuresTrade = append(writer.futuresTrade, measurement)

		return
	}

	writer.measurements = append(writer.measurements, measurement)
}

/*
AddExcursion routes a completed excursion record to the excursions buffer.
*/
func (writer *Writer) AddExcursion(excursion ExcursionRecord) {
	writer.lock()
	defer writer.unlock()

	writer.excursions = append(writer.excursions, excursion)
}

/*
Pending returns total buffered measurements and excursions across all families.
*/
func (writer *Writer) Pending() int {
	writer.lock()
	defer writer.unlock()

	return len(writer.spotTicker) + len(writer.spotTrade) + len(writer.spotLevel3) +
		len(writer.futuresTicker) + len(writer.futuresTrade) +
		len(writer.measurements) + len(writer.excursions)
}

/*
CommitReady commits any family whose buffer meets its volume threshold, or all families if forceAll is true.
*/
func (writer *Writer) CommitReady(ctx context.Context, forceAll bool) error {
	if err := writer.commitFamily(ctx, SpotTicker, tickerBatchThreshold, forceAll, func() []*data.Measurement[float64] {
		rows := writer.spotTicker
		writer.spotTicker = nil

		return rows
	}, func(remaining []*data.Measurement[float64]) {
		writer.spotTicker = append(remaining, writer.spotTicker...)
	}); err != nil {
		return err
	}

	if err := writer.commitFamily(ctx, SpotTrade, tradeBatchThreshold, forceAll, func() []*data.Measurement[float64] {
		rows := writer.spotTrade
		writer.spotTrade = nil

		return rows
	}, func(remaining []*data.Measurement[float64]) {
		writer.spotTrade = append(remaining, writer.spotTrade...)
	}); err != nil {
		return err
	}

	if err := writer.commitFamily(ctx, SpotLevel3, level3BatchThreshold, forceAll, func() []*data.Measurement[float64] {
		rows := writer.spotLevel3
		writer.spotLevel3 = nil

		return rows
	}, func(remaining []*data.Measurement[float64]) {
		writer.spotLevel3 = append(remaining, writer.spotLevel3...)
	}); err != nil {
		return err
	}

	if err := writer.commitFamily(ctx, FuturesTicker, tickerBatchThreshold, forceAll, func() []*data.Measurement[float64] {
		rows := writer.futuresTicker
		writer.futuresTicker = nil

		return rows
	}, func(remaining []*data.Measurement[float64]) {
		writer.futuresTicker = append(remaining, writer.futuresTicker...)
	}); err != nil {
		return err
	}

	if err := writer.commitFamily(ctx, FuturesTrade, tradeBatchThreshold, forceAll, func() []*data.Measurement[float64] {
		rows := writer.futuresTrade
		writer.futuresTrade = nil

		return rows
	}, func(remaining []*data.Measurement[float64]) {
		writer.futuresTrade = append(remaining, writer.futuresTrade...)
	}); err != nil {
		return err
	}

	if err := writer.commitFamily(ctx, Measurements, measurementBatchThreshold, forceAll, func() []*data.Measurement[float64] {
		rows := writer.measurements
		writer.measurements = nil

		return rows
	}, func(remaining []*data.Measurement[float64]) {
		writer.measurements = append(remaining, writer.measurements...)
	}); err != nil {
		return err
	}

	// Outcome references become visible only after every tape family is durable.
	if !forceAll {
		return nil
	}

	if err := writer.commitExcursions(ctx); err != nil {
		return err
	}

	return nil
}

/*
CommitExcursions persists only the excursions buffer. Training durability uses
this path so a hung tape-family Append in Drain cannot keep writing=true while
CommitReady walks empty Spot/Futures/Measurements families.
*/
func (writer *Writer) CommitExcursions(ctx context.Context) error {
	return writer.commitExcursions(ctx)
}

func (writer *Writer) commitExcursions(ctx context.Context) error {
	writer.lock()

	rowsToCommit := writer.excursions
	writer.excursions = nil

	if len(rowsToCommit) == 0 {
		writer.unlock()

		return nil
	}

	writer.unlock()

	tbl, err := writer.catalog.Load(ctx, Excursions)

	if err != nil {
		writer.lock()
		writer.excursions = append(rowsToCommit, writer.excursions...)
		writer.unlock()

		return err
	}

	reader, err := excursionRecords(tbl.Schema(), rowsToCommit, writer.epoch)

	if err != nil {
		writer.lock()
		writer.excursions = append(rowsToCommit, writer.excursions...)
		writer.unlock()

		return err
	}

	defer reader.Release()

	var appendErr error
	if err := writer.catalog.WithCommit(ctx, func() error {
		_, appendErr = tbl.Append(writer.catalog.context(ctx), reader, nil)
		return appendErr
	}); err != nil {
		writer.lock()
		writer.excursions = append(rowsToCommit, writer.excursions...)
		writer.unlock()

		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to append to "+Excursions,
			err,
		))
	}

	return nil
}

func (writer *Writer) commitFamily(
	ctx context.Context,
	tableName string,
	threshold int,
	forceAll bool,
	takeRows func() []*data.Measurement[float64],
	putRows func([]*data.Measurement[float64]),
) error {
	writer.lock()

	rowsToCommit := takeRows()

	if len(rowsToCommit) == 0 {
		writer.unlock()

		return nil
	}

	if !forceAll && len(rowsToCommit) < threshold {
		putRows(rowsToCommit)
		writer.unlock()

		return nil
	}

	writer.unlock()

	tbl, err := writer.catalog.Load(ctx, tableName)

	if err != nil {
		writer.lock()
		putRows(rowsToCommit)
		writer.unlock()

		return err
	}

	reader, err := measurementRecords(tbl.Schema(), rowsToCommit, writer.epoch)

	if err != nil {
		writer.lock()
		putRows(rowsToCommit)
		writer.unlock()

		return err
	}

	defer reader.Release()

	var appendErr error
	if err := writer.catalog.WithCommit(ctx, func() error {
		_, appendErr = tbl.Append(writer.catalog.context(ctx), reader, nil)
		return appendErr
	}); err != nil {
		writer.lock()
		putRows(rowsToCommit)
		writer.unlock()

		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to append to "+tableName,
			err,
		))
	}

	return nil
}
