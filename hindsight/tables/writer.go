package tables

import (
	"context"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
)

const (
	tickerBatchThreshold      = 2000
	tradeBatchThreshold       = 2000
	level3BatchThreshold      = 20000
	measurementBatchThreshold = 2000
)

/*
Writer buffers incoming measurements per canonical table family and commits them
to Iceberg using per-family volume thresholds. This prevents high-volume Level 3
streams from forcing frequent micro-file commits across quiet ticker and trade families.
*/
type Writer struct {
	catalog *Catalog
	epoch   int64

	mutex        sync.Mutex
	spotTicker   []*data.Measurement[float64]
	spotTrade    []*data.Measurement[float64]
	spotLevel3   []*data.Measurement[float64]
	measurements []*data.Measurement[float64]
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

	writer.mutex.Lock()
	defer writer.mutex.Unlock()

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

	writer.measurements = append(writer.measurements, measurement)
}

/*
Pending returns total buffered measurements across all families.
*/
func (writer *Writer) Pending() int {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()

	return len(writer.spotTicker) + len(writer.spotTrade) + len(writer.spotLevel3) + len(writer.measurements)
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

	if err := writer.commitFamily(ctx, Measurements, measurementBatchThreshold, forceAll, func() []*data.Measurement[float64] {
		rows := writer.measurements
		writer.measurements = nil

		return rows
	}, func(remaining []*data.Measurement[float64]) {
		writer.measurements = append(remaining, writer.measurements...)
	}); err != nil {
		return err
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
	writer.mutex.Lock()

	rowsToCommit := takeRows()

	if len(rowsToCommit) == 0 {
		writer.mutex.Unlock()

		return nil
	}

	if !forceAll && len(rowsToCommit) < threshold {
		putRows(rowsToCommit)
		writer.mutex.Unlock()

		return nil
	}

	writer.mutex.Unlock()

	tbl, err := writer.catalog.Load(ctx, tableName)

	if err != nil {
		writer.mutex.Lock()
		putRows(rowsToCommit)
		writer.mutex.Unlock()

		return err
	}

	reader, err := measurementRecords(tbl.Schema(), rowsToCommit, writer.epoch)

	if err != nil {
		writer.mutex.Lock()
		putRows(rowsToCommit)
		writer.mutex.Unlock()

		return err
	}

	defer reader.Release()

	_, appendErr := tbl.Append(ctx, reader, nil)

	if appendErr != nil {
		writer.mutex.Lock()
		putRows(rowsToCommit)
		writer.mutex.Unlock()

		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to append to "+tableName,
			appendErr,
		))
	}

	return nil
}
