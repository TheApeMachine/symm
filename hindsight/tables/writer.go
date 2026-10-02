package tables

import (
	"context"
	"runtime"
	"sync/atomic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
)

const (
	measurementBatchThreshold = 2000
	writerMask                = 1 << 30
)

/*
Writer buffers incoming measurements and commits them to Iceberg.
*/
type Writer struct {
	catalog *Catalog
	epoch   int64

	gate         atomic.Int64
	measurements []data.Publication
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
Add routes an incoming publication to its canonical table family buffer.
*/
func (writer *Writer) Add(channel string, pub data.Publication) {
	if pub.Measurement == nil {
		return
	}

	writer.lock()
	defer writer.unlock()

	writer.measurements = append(writer.measurements, pub)
}

/*
Pending returns total buffered measurements.
*/
func (writer *Writer) Pending() int {
	writer.lock()
	defer writer.unlock()

	return len(writer.measurements)
}

/*
CommitReady commits any family whose buffer meets its volume threshold, or all families if forceAll is true.
*/
func (writer *Writer) CommitReady(ctx context.Context, forceAll bool) error {
	if err := writer.commitFamily(ctx, Measurements, measurementBatchThreshold, forceAll, func() []data.Publication {
		rows := writer.measurements
		writer.measurements = nil

		return rows
	}, func(remaining []data.Publication) {
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
	takeRows func() []data.Publication,
	putRows func([]data.Publication),
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

	measList := make([]*data.Measurement[float64], len(rowsToCommit))
	for idx, pub := range rowsToCommit {
		measList[idx] = pub.Measurement
	}

	reader, err := measurementRecords(tbl.Schema(), measList, writer.epoch)
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

	// Release generations now that storage has committed the batch
	for _, pub := range rowsToCommit {
		pub.Release()
	}

	return nil
}

func (writer *Writer) ReleaseRemaining() {
	writer.lock()
	defer writer.unlock()

	for _, p := range writer.measurements {
		p.Release()
	}
	writer.measurements = nil
}
