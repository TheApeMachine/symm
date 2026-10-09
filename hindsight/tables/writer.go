package tables

import (
	"context"
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
)

const (
	defaultTargetCommitBytes  = 16 * 1024 * 1024 // 16 MB on-disk batch target
	measurementBatchThreshold = 20000
)

/*
Writer buffers incoming measurements and commits them to Iceberg.
*/
type Writer struct {
	catalog *Catalog
	table   string
	epoch   int64

	mu            sync.Mutex
	measurements  []*data.Measurement
	bufferedBytes int64
}

func (writer *Writer) lock() {
	writer.mu.Lock()
}

func (writer *Writer) unlock() {
	writer.mu.Unlock()
}

/*
NewWriter constructs a Writer writing into the given catalog for a stable run epoch.
*/
func NewWriter(catalog *Catalog, epoch int64) *Writer {
	return &Writer{
		catalog: catalog,
		table:   Measurements,
		epoch:   epoch,
	}
}

/*
Add routes an incoming publication to its canonical table family buffer.
*/
func (writer *Writer) Add(channel string, measurement *data.Measurement) {
	if measurement == nil {
		return
	}

	writer.lock()
	defer writer.unlock()

	writer.measurements = append(writer.measurements, measurement)
	writer.bufferedBytes += measurement.ApproximateBytes()
}

/*
CommitReady commits any family whose buffer meets its volume or size threshold, or all families if forceAll is true.
*/
func (writer *Writer) CommitReady(ctx context.Context, forceAll bool) error {
	targetBytes := int64(defaultTargetCommitBytes)

	if configured := viper.GetInt64("hindsight.capture.commit_bytes"); configured > 0 {
		targetBytes = configured
	}

	threshold := measurementBatchThreshold

	if viper.GetInt("hindsight.capture.commit_rows") > 0 {
		threshold = viper.GetInt("hindsight.capture.commit_rows")
	}

	if err := writer.commitFamily(ctx, writer.table, targetBytes, threshold, forceAll, func() ([]*data.Measurement, int64) {
		rows := writer.measurements
		bytes := writer.bufferedBytes
		writer.measurements = nil
		writer.bufferedBytes = 0

		return rows, bytes
	}, func(remaining []*data.Measurement, remainingBytes int64) {
		writer.measurements = append(remaining, writer.measurements...)
		writer.bufferedBytes += remainingBytes
	}); err != nil {
		return err
	}

	return nil
}

func (writer *Writer) commitFamily(
	ctx context.Context,
	tableName string,
	targetBytes int64,
	thresholdRows int,
	forceAll bool,
	takeRows func() ([]*data.Measurement, int64),
	putRows func([]*data.Measurement, int64),
) error {
	writer.lock()

	rowsToCommit, totalBytes := takeRows()

	if len(rowsToCommit) == 0 {
		writer.unlock()

		return nil
	}

	if !forceAll && totalBytes < targetBytes && len(rowsToCommit) < thresholdRows {
		putRows(rowsToCommit, totalBytes)
		writer.unlock()

		return nil
	}

	writer.unlock()

	tbl, err := writer.catalog.Load(ctx, tableName)

	if err != nil {
		writer.lock()
		putRows(rowsToCommit, totalBytes)
		writer.unlock()

		return err
	}

	measList := make([]*data.Measurement, len(rowsToCommit))
	copy(measList, rowsToCommit)

	reader, err := measurementRecords(tbl.Schema(), measList, writer.epoch)
	if err != nil {
		writer.lock()
		putRows(rowsToCommit, totalBytes)
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
		putRows(rowsToCommit, totalBytes)
		writer.unlock()

		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to append to "+tableName,
			err,
		))
	}

	return nil
}

func (writer *Writer) ReleaseRemaining() {
	writer.lock()
	defer writer.unlock()

	writer.measurements = nil
	writer.bufferedBytes = 0
}

/*
BufferedRows returns the current count of buffered measurements waiting to be committed.
*/
func (writer *Writer) BufferedRows() int {
	writer.lock()
	defer writer.unlock()

	return len(writer.measurements)
}

/*
BufferedBytes returns the approximate memory footprint in bytes of buffered measurements.
*/
func (writer *Writer) BufferedBytes() int64 {
	writer.lock()
	defer writer.unlock()

	return writer.bufferedBytes
}

/*
Append commits measurements for one epoch to the named table as a single
snapshot. Nothing is retained on success; on failure the error is returned and
the rows are not committed.
*/
func (catalog *Catalog) Append(
	ctx context.Context, tableName string, epoch int64, measurements []*data.Measurement,
) error {
	if catalog == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[catalog] catalog is required", nil,
		))
	}

	if epoch <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[catalog] valid positive epoch is required", nil,
		))
	}

	if len(measurements) == 0 {
		return nil
	}

	writer := NewWriter(catalog, epoch)
	writer.table = tableName
	defer writer.ReleaseRemaining()

	for _, measurement := range measurements {
		if measurement == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation, "[catalog] cannot append a nil measurement", nil,
			))
		}

		if measurement.Epoch != epoch {
			return errnie.Error(errnie.Err(
				errnie.Validation, "[catalog] measurement epoch does not match append epoch", nil,
			))
		}

		writer.Add(tableName, measurement)
	}

	return errnie.Error(writer.CommitReady(ctx, true))
}
